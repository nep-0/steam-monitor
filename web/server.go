// Package web exposes the standalone browser UI and JSON API.
package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"

	"steam-monitor/monitor"
	"steam-monitor/steam"
	"steam-monitor/store"
)

//go:embed index.html app.js style.css
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
	assets, _ := fs.Sub(files, ".")
	mux.Handle("/", http.FileServer(http.FS(assets)))
	mux.HandleFunc("/api/health", s.health)
	mux.HandleFunc("/api/players", s.players)
	mux.HandleFunc("/api/players/", s.playerByID)
	mux.HandleFunc("/api/dashboard", s.dashboard)
	mux.HandleFunc("/api/sessions", s.sessions)
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
func (s *Server) playerByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.WriteHeader(405)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/players/")
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
func integer(r *http.Request, fallback, min, max int) int {
	n, err := strconv.Atoi(r.URL.Query().Get("days"))
	if err != nil || n < min || n > max {
		return fallback
	}
	return n
}
