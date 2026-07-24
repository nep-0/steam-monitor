// Package monitor coordinates scheduled Steam polling and session recording.
package monitor

import (
	"context"
	"log"
	"sync"
	"time"

	"steam-monitor/steam"
	"steam-monitor/store"
)

type Monitor struct {
	store         *store.Store
	steam         *steam.Client
	interval      time.Duration
	retentionDays int
	mu            sync.Mutex
	lastPoll      time.Time
	lastError     string
}

func New(s *store.Store, c *steam.Client, interval time.Duration, retentionDays int) *Monitor {
	return &Monitor{store: s, steam: c, interval: interval, retentionDays: retentionDays}
}
func (m *Monitor) Start(ctx context.Context) {
	go func() {
		_ = m.Poll(ctx)
		ticker := time.NewTicker(m.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = m.Poll(ctx)
			}
		}
	}()
}
func (m *Monitor) Poll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	started := time.Now()
	tracked, err := m.store.TrackedPlayers()
	if err != nil {
		return m.failed(err)
	}
	log.Printf("poll started: tracked_players=%d retention_days=%d", len(tracked), m.retentionDays)
	ids := make([]string, 0, len(tracked))
	prior := make(map[string]store.TrackedPlayer, len(tracked))
	for _, p := range tracked {
		ids = append(ids, p.SteamID)
		prior[p.SteamID] = p
	}
	if len(ids) > 0 {
		players, err := m.steam.Players(ctx, ids)
		if err != nil {
			return m.failed(err)
		}
		if err := m.store.ApplyPoll(players, prior, time.Now(), m.retentionDays); err != nil {
			return m.failed(err)
		}
		log.Printf("poll Steam response: requested=%d returned=%d", len(ids), len(players))
	}
	m.lastPoll = time.Now()
	m.lastError = ""
	log.Printf("poll completed: tracked_players=%d duration=%s", len(ids), time.Since(started).Round(time.Millisecond))
	return nil
}
func (m *Monitor) failed(err error) error {
	m.lastError = err.Error()
	log.Printf("poll failed: %v", err)
	return err
}
func (m *Monitor) Health() (time.Time, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastPoll, m.lastError
}
