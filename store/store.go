// Package store owns the SQLite schema and persistence operations.
package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }
type Player struct {
	SteamID   string `json:"steam_id"`
	Nickname  string `json:"nickname"`
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
	Ongoing bool   `json:"ongoing"`
}
type Game struct {
	Name    string `json:"name"`
	Seconds int64  `json:"seconds"`
}
type DailyActivity struct {
	Date    string `json:"date"`
	Seconds int64  `json:"seconds"`
}
type PlayerActivity struct {
	SteamID string `json:"steam_id"`
	Player  string `json:"player"`
	Seconds int64  `json:"seconds"`
}

type GanttPlayer struct {
	SteamID  string    `json:"steam_id"`
	Player   string    `json:"player"`
	Sessions []Session `json:"sessions"`
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
func (s *Store) UpdateNickname(id, nickname string) (bool, error) {
	result, err := s.db.Exec(`UPDATE players SET nickname=? WHERE steam_id=?`, nickname, id)
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	return changed > 0, err
}
func (s *Store) DeletePlayer(id string) error {
	_, err := s.db.Exec(`DELETE FROM players WHERE steam_id=?`, id)
	return err
}
func (s *Store) Players() ([]Player, error) {
	rows, err := s.db.Query(`SELECT steam_id,nickname,COALESCE(NULLIF(nickname,''),name,steam_id),avatar,personastate,game_id,game_name,updated_at FROM players ORDER BY 3 COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Player, 0)
	for rows.Next() {
		var p Player
		if err := rows.Scan(&p.SteamID, &p.Nickname, &p.Name, &p.Avatar, &p.State, &p.GameID, &p.Game, &p.UpdatedAt); err != nil {
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
	for steamID, old := range prior {
		if _, ok := seen[steamID]; ok {
			continue
		}
		if old.GameID > 0 && old.GameStartedAt > 0 && now.Unix() > old.GameStartedAt {
			if _, err = tx.Exec(`INSERT INTO sessions(steam_id,game_id,game_name,started_at,ended_at,duration_seconds) VALUES(?,?,?,?,?,?)`, steamID, old.GameID, old.Game, old.GameStartedAt, now.Unix(), now.Unix()-old.GameStartedAt); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(`UPDATE players SET personastate=0,game_id=0,game_name='',game_started_at=0,updated_at=? WHERE steam_id=?`, now.Unix(), steamID); err != nil {
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
func (s *Store) PlayerSessions(steamID string, days int) ([]Session, error) {
	rows, err := s.db.Query(`SELECT s.steam_id, COALESCE(NULLIF(p.nickname,''),p.name,s.steam_id), s.game_name, s.started_at, s.ended_at, s.duration_seconds FROM sessions s LEFT JOIN players p ON p.steam_id=s.steam_id WHERE s.steam_id=? AND s.ended_at>=? ORDER BY s.started_at DESC LIMIT 1000`, steamID, time.Now().AddDate(0, 0, -days).Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Session, 0)
	for rows.Next() {
		var item Session
		if err := rows.Scan(&item.SteamID, &item.Player, &item.Game, &item.Started, &item.Ended, &item.Seconds); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
func (s *Store) SearchPlayers(query string) ([]Player, error) {
	pattern := "%" + query + "%"
	rows, err := s.db.Query(`SELECT steam_id,nickname,COALESCE(NULLIF(nickname,''),name,steam_id),avatar,personastate,game_id,game_name,updated_at FROM players WHERE steam_id LIKE ? OR nickname LIKE ? OR name LIKE ? ORDER BY 3 COLLATE NOCASE LIMIT 50`, pattern, pattern, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Player, 0)
	for rows.Next() {
		var p Player
		if err := rows.Scan(&p.SteamID, &p.Nickname, &p.Name, &p.Avatar, &p.State, &p.GameID, &p.Game, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Store) Analytics(days int) ([]DailyActivity, []PlayerActivity, error) {
	since := time.Now().AddDate(0, 0, -days).Unix()
	dailyRows, err := s.db.Query(`SELECT date(ended_at, 'unixepoch', 'localtime'), SUM(duration_seconds) FROM sessions WHERE ended_at >= ? GROUP BY 1 ORDER BY 1`, since)
	if err != nil {
		return nil, nil, err
	}
	defer dailyRows.Close()
	daily := make([]DailyActivity, 0)
	for dailyRows.Next() {
		var item DailyActivity
		if err := dailyRows.Scan(&item.Date, &item.Seconds); err != nil {
			return nil, nil, err
		}
		daily = append(daily, item)
	}
	if err := dailyRows.Err(); err != nil {
		return nil, nil, err
	}
	playerRows, err := s.db.Query(`SELECT s.steam_id, COALESCE(NULLIF(p.nickname,''), p.name, s.steam_id), SUM(s.duration_seconds) FROM sessions s LEFT JOIN players p ON p.steam_id=s.steam_id WHERE s.ended_at >= ? GROUP BY s.steam_id ORDER BY 3 DESC LIMIT 25`, since)
	if err != nil {
		return nil, nil, err
	}
	defer playerRows.Close()
	players := make([]PlayerActivity, 0)
	for playerRows.Next() {
		var item PlayerActivity
		if err := playerRows.Scan(&item.SteamID, &item.Player, &item.Seconds); err != nil {
			return nil, nil, err
		}
		players = append(players, item)
	}
	return daily, players, playerRows.Err()
}

func (s *Store) Gantt(days int, offset int) ([]GanttPlayer, time.Time, time.Time, error) {
	if days < 1 {
		days = 1
	}
	if offset < 0 {
		offset = 0
	}
	now := time.Now()
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1-offset)
	start := end.AddDate(0, 0, -days)
	items, err := s.Sessions(days + offset + 1)
	if err != nil {
		return nil, time.Time{}, time.Time{}, err
	}
	byPlayer := make(map[string]*GanttPlayer)
	for _, item := range items {
		if item.Ended <= start.Unix() || item.Started >= end.Unix() {
			continue
		}
		if item.Started < start.Unix() {
			item.Seconds -= start.Unix() - item.Started
			item.Started = start.Unix()
		}
		if item.Ended > end.Unix() {
			item.Seconds -= item.Ended - end.Unix()
			item.Ended = end.Unix()
		}
		entry := byPlayer[item.SteamID]
		if entry == nil {
			entry = &GanttPlayer{SteamID: item.SteamID, Player: item.Player, Sessions: make([]Session, 0)}
			byPlayer[item.SteamID] = entry
		}
		entry.Sessions = append(entry.Sessions, item)
	}
	activeRows, err := s.db.Query(`SELECT steam_id,COALESCE(NULLIF(nickname,''),name,steam_id),game_name,game_started_at FROM players WHERE game_id>0 AND game_started_at>0`)
	if err != nil {
		return nil, time.Time{}, time.Time{}, err
	}
	defer activeRows.Close()
	activeEnd := now.Unix()
	if activeEnd > end.Unix() {
		activeEnd = end.Unix()
	}
	for activeRows.Next() {
		var steamID, player, game string
		var started int64
		if err := activeRows.Scan(&steamID, &player, &game, &started); err != nil {
			return nil, time.Time{}, time.Time{}, err
		}
		if activeEnd <= start.Unix() || started >= end.Unix() {
			continue
		}
		if started < start.Unix() {
			started = start.Unix()
		}
		if activeEnd <= started {
			continue
		}
		entry := byPlayer[steamID]
		if entry == nil {
			entry = &GanttPlayer{SteamID: steamID, Player: player, Sessions: make([]Session, 0)}
			byPlayer[steamID] = entry
		}
		entry.Sessions = append(entry.Sessions, Session{SteamID: steamID, Player: player, Game: game, Started: started, Ended: activeEnd, Seconds: activeEnd - started, Ongoing: true})
	}
	if err := activeRows.Err(); err != nil {
		return nil, time.Time{}, time.Time{}, err
	}
	out := make([]GanttPlayer, 0, len(byPlayer))
	for _, item := range byPlayer {
		sort.Slice(item.Sessions, func(i, j int) bool { return item.Sessions[i].Started < item.Sessions[j].Started })
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Player < out[j].Player })
	return out, start, end, nil
}

func (s *Store) Heatmap(days int, steamID string) (map[string]int64, error) {
	if days < 1 {
		days = 1
	}
	end := time.Now()
	start := end.AddDate(0, 0, -days)
	query := `SELECT started_at, ended_at FROM sessions WHERE ended_at > ?`
	args := []any{start.Unix()}
	if steamID != "" {
		query += ` AND steam_id = ?`
		args = append(args, steamID)
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]int64, days+1)
	firstDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	for day := firstDay; !day.After(end); day = day.AddDate(0, 0, 1) {
		result[day.Format("2006-01-02")] = 0
	}
	for rows.Next() {
		var started, ended int64
		if err := rows.Scan(&started, &ended); err != nil {
			return nil, err
		}
		if ended <= started || started >= end.Unix() {
			continue
		}
		if started < start.Unix() {
			started = start.Unix()
		}
		if ended > end.Unix() {
			ended = end.Unix()
		}
		for cursor := time.Unix(started, 0).In(time.Local); cursor.Unix() < ended; {
			nextDay := time.Date(cursor.Year(), cursor.Month(), cursor.Day()+1, 0, 0, 0, 0, cursor.Location())
			segmentEnd := ended
			if nextDay.Unix() < segmentEnd {
				segmentEnd = nextDay.Unix()
			}
			result[cursor.Format("2006-01-02")] += segmentEnd - cursor.Unix()
			cursor = time.Unix(segmentEnd, 0).In(time.Local)
		}
	}
	return result, rows.Err()
}
