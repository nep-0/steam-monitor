// Package web exposes the standalone browser UI and JSON API.
package web

import (
	"embed"
	"encoding/csv"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"steam-monitor/monitor"
	"steam-monitor/steam"
	"steam-monitor/store"
)

//go:embed dist/*
var files embed.FS

type Server struct {
	store   *store.Store
	steam   *steam.Client
	monitor *monitor.Monitor
}

func New(s *store.Store, c *steam.Client, m *monitor.Monitor) *Server {
	return &Server{store: s, steam: c, monitor: m}
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	assets, _ := fs.Sub(files, "dist")
	mux.Handle("/", http.FileServer(http.FS(assets)))
	mux.HandleFunc("/api/health", s.health)
	mux.HandleFunc("/api/players", s.players)
	mux.HandleFunc("/api/players/", s.playerByID)
	mux.HandleFunc("/api/players/search", s.searchPlayers)
	mux.HandleFunc("/api/dashboard", s.dashboard)
	mux.HandleFunc("/api/sessions", s.sessions)
	mux.HandleFunc("/api/analytics", s.analytics)
	mux.HandleFunc("/api/gantt", s.gantt)
	mux.HandleFunc("/api/heatmap", s.heatmap)
	mux.HandleFunc("/api/export/sessions.csv", s.exportSessions)
	mux.HandleFunc("/api/export/sessions.json", s.exportSessionsJSON)
	mux.HandleFunc("/api/poll", s.poll)
	return mux
}
func out(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	last, err := s.monitor.Health()
	out(w, 200, map[string]any{"last_poll": last, "last_error": err})
}
func (s *Server) players(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		players, err := s.store.Players()
		if err != nil {
			out(w, 500, map[string]string{"error": err.Error()})
			return
		}
		out(w, 200, players)
	case http.MethodPost:
		var in struct {
			SteamID  string `json:"steam_id"`
			Nickname string `json:"nickname"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			out(w, 400, map[string]string{"error": "invalid JSON"})
			return
		}
		items := strings.FieldsFunc(in.SteamID, func(r rune) bool { return r == ',' || r == '，' || r == '\n' })
		if len(items) == 0 {
			out(w, 400, map[string]string{"error": "enter at least one Steam ID or profile URL"})
			return
		}
		var ids []string
		for _, item := range items {
			id, err := s.steam.Resolve(r.Context(), item)
			if err != nil {
				out(w, 400, map[string]string{"error": err.Error()})
				return
			}
			if err := s.store.AddPlayer(id, strings.TrimSpace(in.Nickname)); err != nil {
				out(w, 500, map[string]string{"error": err.Error()})
				return
			}
			ids = append(ids, id)
		}
		log.Printf("web add players: requested=%d resolved=%d", len(items), len(ids))
		out(w, 201, map[string]any{"steam_ids": ids})
	default:
		w.WriteHeader(405)
	}
}
func (s *Server) searchPlayers(w http.ResponseWriter, r *http.Request) {
	players, err := s.store.SearchPlayers(strings.TrimSpace(r.URL.Query().Get("q")))
	if err != nil {
		out(w, 500, map[string]string{"error": err.Error()})
		return
	}
	out(w, 200, players)
}
func (s *Server) playerByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/players/")
	if r.Method == http.MethodPatch {
		if len(id) != 17 {
			out(w, 400, map[string]string{"error": "invalid SteamID64"})
			return
		}
		var in struct {
			Nickname string `json:"nickname"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			out(w, 400, map[string]string{"error": "invalid JSON"})
			return
		}
		in.Nickname = strings.TrimSpace(in.Nickname)
		if len([]rune(in.Nickname)) > 80 {
			out(w, 400, map[string]string{"error": "nickname must be 80 characters or fewer"})
			return
		}
		found, err := s.store.UpdateNickname(id, in.Nickname)
		if err != nil {
			out(w, 500, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			out(w, 404, map[string]string{"error": "player not found"})
			return
		}
		log.Printf("web update nickname: steam_id=%s nickname_set=%t", id, in.Nickname != "")
		out(w, 200, map[string]string{"steam_id": id, "nickname": in.Nickname})
		return
	}
	if r.Method == http.MethodGet {
		days := integer(r, 90, 1, 3650)
		players, err := s.store.Players()
		if err != nil {
			out(w, 500, map[string]string{"error": err.Error()})
			return
		}
		var found any
		for _, player := range players {
			if player.SteamID == id {
				found = player
				break
			}
		}
		if found == nil {
			out(w, 404, map[string]string{"error": "player not found"})
			return
		}
		sessions, err := s.store.PlayerSessions(id, days)
		if err != nil {
			out(w, 500, map[string]string{"error": err.Error()})
			return
		}
		out(w, 200, map[string]any{"player": found, "sessions": sessions, "days": days})
		return
	}
	if r.Method != http.MethodDelete {
		w.WriteHeader(405)
		return
	}
	if len(id) != 17 {
		out(w, 400, map[string]string{"error": "invalid SteamID64"})
		return
	}
	if err := s.store.DeletePlayer(id); err != nil {
		out(w, 500, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(204)
}
func (s *Server) poll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	if err := s.monitor.Poll(r.Context()); err != nil {
		out(w, 502, map[string]string{"error": err.Error()})
		return
	}
	out(w, 200, map[string]bool{"ok": true})
}
func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	days := integer(r, 7, 1, 3650)
	total, playing, games, err := s.store.Dashboard(days)
	if err != nil {
		out(w, 500, map[string]string{"error": err.Error()})
		return
	}
	out(w, 200, map[string]any{"total_players": total, "playing": playing, "top_games": games})
}
func (s *Server) sessions(w http.ResponseWriter, r *http.Request) {
	days := integer(r, 30, 1, 3650)
	sessions, err := s.store.Sessions(days)
	if err != nil {
		out(w, 500, map[string]string{"error": err.Error()})
		return
	}
	out(w, 200, sessions)
}
func (s *Server) analytics(w http.ResponseWriter, r *http.Request) {
	days := integer(r, 90, 1, 3650)
	daily, players, err := s.store.Analytics(days)
	if err != nil {
		out(w, 500, map[string]string{"error": err.Error()})
		return
	}
	out(w, 200, map[string]any{"days": days, "daily": daily, "players": players})
}
func (s *Server) gantt(w http.ResponseWriter, r *http.Request) {
	days := integer(r, 1, 1, 90)
	offset := integerParam(r, "offset", 0, 0, 3650)
	players, start, end, err := s.store.Gantt(days, offset)
	if err != nil {
		out(w, 500, map[string]string{"error": err.Error()})
		return
	}
	out(w, 200, map[string]any{"players": players, "time_range": map[string]string{"start": start.Format(time.RFC3339), "end": end.Format(time.RFC3339)}})
}
func (s *Server) heatmap(w http.ResponseWriter, r *http.Request) {
	days := integer(r, 90, 1, 3650)
	values, err := s.store.Heatmap(days, strings.TrimSpace(r.URL.Query().Get("player")))
	if err != nil {
		out(w, 500, map[string]string{"error": err.Error()})
		return
	}
	out(w, 200, map[string]any{"days": days, "heatmap": values})
}
func (s *Server) exportSessions(w http.ResponseWriter, r *http.Request) {
	days := integer(r, 30, 1, 3650)
	sessions, err := s.store.Sessions(days)
	if err != nil {
		out(w, 500, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=steam-sessions.csv")
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"steam_id", "player", "game", "started_at", "ended_at", "seconds"})
	for _, session := range sessions {
		_ = writer.Write([]string{session.SteamID, session.Player, session.Game, strconv.FormatInt(session.Started, 10), strconv.FormatInt(session.Ended, 10), strconv.FormatInt(session.Seconds, 10)})
	}
	writer.Flush()
}
func (s *Server) exportSessionsJSON(w http.ResponseWriter, r *http.Request) {
	days := integer(r, 30, 1, 3650)
	sessions, err := s.store.Sessions(days)
	if err != nil {
		out(w, 500, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=steam-sessions.json")
	out(w, 200, sessions)
}
func integer(r *http.Request, fallback, min, max int) int {
	n, err := strconv.Atoi(r.URL.Query().Get("days"))
	if err != nil || n < min || n > max {
		return fallback
	}
	return n
}
func integerParam(r *http.Request, key string, fallback, min, max int) int {
	n, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil || n < min || n > max {
		return fallback
	}
	return n
}
