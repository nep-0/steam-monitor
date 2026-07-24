// Package store owns the SQLite schema and persistence operations.
package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }
type Player struct {
	SteamID   string `json:"steam_id"`
	Name      string `json:"name"`
	Avatar    string `json:"avatar"`
	State     int    `json:"state"`
	GameID    int64  `json:"game_id"`
	Game      string `json:"game"`
	UpdatedAt int64  `json:"updated_at"`
}
type TrackedPlayer struct {
	SteamID       string
	GameID        int64
	Game          string
	GameStartedAt int64
}
type Session struct {
	SteamID string `json:"steam_id"`
	Player  string `json:"player"`
	Game    string `json:"game"`
	Started int64  `json:"started_at"`
	Ended   int64  `json:"ended_at"`
	Seconds int64  `json:"seconds"`
}
type Game struct {
	Name    string `json:"name"`
	Seconds int64  `json:"seconds"`
}

func Open(path string) (*Store, error) {
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) migrate() error {
	_, err := s.db.Exec(`PRAGMA journal_mode=WAL;
CREATE TABLE IF NOT EXISTS players (steam_id TEXT PRIMARY KEY, nickname TEXT NOT NULL DEFAULT '', name TEXT NOT NULL DEFAULT '', avatar TEXT NOT NULL DEFAULT '', personastate INTEGER NOT NULL DEFAULT 0, game_id INTEGER NOT NULL DEFAULT 0, game_name TEXT NOT NULL DEFAULT '', game_started_at INTEGER NOT NULL DEFAULT 0, updated_at INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS sessions (id INTEGER PRIMARY KEY, steam_id TEXT NOT NULL, game_id INTEGER NOT NULL, game_name TEXT NOT NULL, started_at INTEGER NOT NULL, ended_at INTEGER NOT NULL, duration_seconds INTEGER NOT NULL);
CREATE INDEX IF NOT EXISTS sessions_ended_at ON sessions(ended_at);`)
	if err != nil {
		return err
	}
	_, _ = s.db.Exec(`ALTER TABLE players ADD COLUMN game_started_at INTEGER NOT NULL DEFAULT 0`)
	return nil
}
func (s *Store) AddPlayer(id, nickname string) error {
	_, err := s.db.Exec(`INSERT INTO players(steam_id,nickname) VALUES(?,?) ON CONFLICT(steam_id) DO UPDATE SET nickname=excluded.nickname`, id, nickname)
	return err
}
func (s *Store) DeletePlayer(id string) error {
	_, err := s.db.Exec(`DELETE FROM players WHERE steam_id=?`, id)
	return err
}
func (s *Store) Players() ([]Player, error) {
	rows, err := s.db.Query(`SELECT steam_id,COALESCE(NULLIF(nickname,''),name,steam_id),avatar,personastate,game_id,game_name,updated_at FROM players ORDER BY 2 COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Player, 0)
	for rows.Next() {
		var p Player
		if err := rows.Scan(&p.SteamID, &p.Name, &p.Avatar, &p.State, &p.GameID, &p.Game, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Store) TrackedPlayers() ([]TrackedPlayer, error) {
	rows, err := s.db.Query(`SELECT steam_id,game_id,game_name,game_started_at FROM players`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]TrackedPlayer, 0)
	for rows.Next() {
		var p TrackedPlayer
		if err := rows.Scan(&p.SteamID, &p.GameID, &p.Game, &p.GameStartedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Store) ApplyPoll(players []Player, prior map[string]TrackedPlayer, now time.Time, retentionDays int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	seen := map[string]bool{}
	for _, p := range players {
		seen[p.SteamID] = true
		old := prior[p.SteamID]
		if old.GameID > 0 && old.GameID != p.GameID && old.GameStartedAt > 0 && now.Unix() > old.GameStartedAt {
			if _, err = tx.Exec(`INSERT INTO sessions(steam_id,game_id,game_name,started_at,ended_at,duration_seconds) VALUES(?,?,?,?,?,?)`, p.SteamID, old.GameID, old.Game, old.GameStartedAt, now.Unix(), now.Unix()-old.GameStartedAt); err != nil {
				return err
			}
		}
		started := old.GameStartedAt
		if old.GameID != p.GameID {
			started = now.Unix()
		}
		if _, err = tx.Exec(`UPDATE players SET name=?,avatar=?,personastate=?,game_id=?,game_name=?,game_started_at=?,updated_at=? WHERE steam_id=?`, p.Name, p.Avatar, p.State, p.GameID, p.Game, started, now.Unix(), p.SteamID); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`DELETE FROM sessions WHERE ended_at < ?`, now.AddDate(0, 0, -retentionDays).Unix()); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Dashboard(days int) (int, int, []Game, error) {
	var total, playing int
	if err := s.db.QueryRow(`SELECT COUNT(*),COALESCE(SUM(CASE WHEN game_id>0 THEN 1 ELSE 0 END),0) FROM players`).Scan(&total, &playing); err != nil {
		return 0, 0, nil, err
	}
	rows, err := s.db.Query(`SELECT game_name,SUM(duration_seconds) FROM sessions WHERE ended_at>=? GROUP BY game_id,game_name ORDER BY 2 DESC LIMIT 10`, time.Now().AddDate(0, 0, -days).Unix())
	if err != nil {
		return 0, 0, nil, err
	}
	defer rows.Close()
	games := make([]Game, 0)
	for rows.Next() {
		var g Game
		if err := rows.Scan(&g.Name, &g.Seconds); err != nil {
			return 0, 0, nil, err
		}
		games = append(games, g)
	}
	return total, playing, games, rows.Err()
}
func (s *Store) Sessions(days int) ([]Session, error) {
	rows, err := s.db.Query(`SELECT s.steam_id,COALESCE(NULLIF(p.nickname,''),p.name,s.steam_id),s.game_name,s.started_at,s.ended_at,s.duration_seconds FROM sessions s LEFT JOIN players p ON p.steam_id=s.steam_id WHERE s.ended_at>=? ORDER BY s.ended_at DESC LIMIT 1000`, time.Now().AddDate(0, 0, -days).Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Session, 0)
	for rows.Next() {
		var x Session
		if err := rows.Scan(&x.SteamID, &x.Player, &x.Game, &x.Started, &x.Ended, &x.Seconds); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
