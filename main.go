package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"time"

	"steam-monitor/config"
	"steam-monitor/monitor"
	"steam-monitor/steam"
	"steam-monitor/store"
	"steam-monitor/web"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	configPath := flag.String("config", "config.json", "path to JSON configuration")
	flag.Parse()
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	client, err := steam.New(cfg.SteamAPIKey, cfg.SteamAPIBase, cfg.ProxyURL, time.Duration(cfg.RequestTimeoutSeconds)*time.Second)
	if err != nil {
		log.Fatal(err)
	}
	service := monitor.New(db, client, time.Duration(cfg.PollIntervalSeconds)*time.Second, cfg.RetentionDays)
	service.Start(context.Background())
	log.Printf("Steam Monitor starting: listen=%s database=%s poll_interval=%s retention_days=%d proxy_enabled=%t", cfg.ListenAddress, cfg.DatabasePath, time.Duration(cfg.PollIntervalSeconds)*time.Second, cfg.RetentionDays, cfg.ProxyURL != "")
	log.Fatal(http.ListenAndServe(cfg.ListenAddress, web.New(db, client, service).Handler()))
}
