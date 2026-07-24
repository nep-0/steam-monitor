package store

import (
	"testing"
	"time"
)

func TestApplyPollClosesMissingPlayingPlayer(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.AddPlayer("76561198000000000", "Test"); err != nil {
		t.Fatal(err)
	}
	started := time.Now().Add(-10 * time.Minute)
	prior := map[string]TrackedPlayer{"76561198000000000": {SteamID: "76561198000000000", GameID: 10, Game: "Example", GameStartedAt: started.Unix()}}
	if err := s.ApplyPoll(nil, prior, time.Now(), 30); err != nil {
		t.Fatal(err)
	}
	sessions, err := s.Sessions(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].Game != "Example" || sessions[0].Seconds < 1 {
		t.Fatalf("expected closed session, got %#v", sessions)
	}
	players, err := s.Players()
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 1 || players[0].GameID != 0 || players[0].State != 0 {
		t.Fatalf("expected offline player, got %#v", players)
	}
}

func TestAnalyticsReturnsNonNilEmptySlices(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	daily, players, err := s.Analytics(30)
	if err != nil {
		t.Fatal(err)
	}
	if daily == nil || players == nil {
		t.Fatalf("expected non-nil analytics slices: daily=%#v players=%#v", daily, players)
	}
}

func TestHeatmapSplitsSessionsAtMidnight(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	started := time.Now().In(time.Local).AddDate(0, 0, -1)
	started = time.Date(started.Year(), started.Month(), started.Day(), 23, 30, 0, 0, started.Location())
	ended := started.Add(time.Hour)
	if _, err := s.db.Exec(`INSERT INTO sessions(steam_id,game_id,game_name,started_at,ended_at,duration_seconds) VALUES(?,?,?,?,?,?)`, "76561198000000000", 10, "Example", started.Unix(), ended.Unix(), 3600); err != nil {
		t.Fatal(err)
	}
	values, err := s.Heatmap(2, "")
	if err != nil {
		t.Fatal(err)
	}
	if values[started.Format("2006-01-02")] != 1800 || values[ended.Format("2006-01-02")] != 1800 {
		t.Fatalf("expected session split across midnight, got %#v", values)
	}
}

func TestUpdateNickname(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	id := "76561198000000000"
	if err := s.AddPlayer(id, "Old name"); err != nil {
		t.Fatal(err)
	}
	found, err := s.UpdateNickname(id, "New name")
	if err != nil || !found {
		t.Fatalf("update nickname: found=%t err=%v", found, err)
	}
	players, err := s.Players()
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 1 || players[0].Nickname != "New name" || players[0].Name != "New name" {
		t.Fatalf("unexpected nickname result: %#v", players)
	}
	if found, err := s.UpdateNickname("76561198000000001", "Missing"); err != nil || found {
		t.Fatalf("missing player update: found=%t err=%v", found, err)
	}
}
