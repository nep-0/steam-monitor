// Package config loads and validates the application's JSON configuration.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

type Config struct {
	SteamAPIKey           string `json:"steam_api_key"`
	ListenAddress         string `json:"listen_address"`
	DatabasePath          string `json:"database_path"`
	PollIntervalSeconds   int    `json:"poll_interval_seconds"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
	RetentionDays         int    `json:"retention_days"`
	ProxyURL              string `json:"proxy_url"`
	SteamAPIBase          string `json:"steam_api_base"`
}

func Load(path string) (Config, error) {
	c := Config{ListenAddress: "127.0.0.1:8080", DatabasePath: "data/steam-monitor.db", PollIntervalSeconds: 60, RequestTimeoutSeconds: 15, RetentionDays: 180, SteamAPIBase: "https://api.steampowered.com"}
	b, err := os.ReadFile(path)
	if err != nil {
		return c, fmt.Errorf("read config %q: %w", path, err)
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("parse config: %w", err)
	}
	if c.SteamAPIKey == "" {
		return c, errors.New("steam_api_key is required")
	}
	if c.PollIntervalSeconds < 15 {
		return c, errors.New("poll_interval_seconds must be at least 15")
	}
	if c.RetentionDays < 1 {
		return c, errors.New("retention_days must be at least 1")
	}
	return c, nil
}
