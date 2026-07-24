package steam

import (
	"context"
	"testing"
	"time"
)

func TestResolveLocalFormats(t *testing.T) {
	c, err := New("key", "https://api.steampowered.com", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	const id = "76561198000000000"
	for _, input := range []string{
		id,
		"https://steamcommunity.com/profiles/" + id,
		"https://s.team/p/" + id,
		"12345678",
	} {
		got, err := c.Resolve(context.Background(), input)
		if err != nil {
			t.Errorf("Resolve(%q): %v", input, err)
			continue
		}
		if input == "12345678" {
			if got != "76561197972611406" {
				t.Errorf("account ID converted to %s", got)
			}
		} else if got != id {
			t.Errorf("Resolve(%q) = %s, want %s", input, got, id)
		}
	}
}
