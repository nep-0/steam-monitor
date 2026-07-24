# Steam Monitor

A standalone local web UI for monitoring public Steam profiles. It polls the Steam Web API, records completed game sessions in SQLite, and removes session history older than the configured retention period. It has no AstrBot, chat, or notification dependencies.

This project is a standalone Go adaptation of [Maoer233/astrbot_plugin_steam_status_monitor](https://github.com/Maoer233/astrbot_plugin_steam_status_monitor), the original AstrBot Steam status monitor plugin.

## Run

1. Download the binary for your OS from [nep-0/steam-monitor releases](https://github.com/nep-0/steam-monitor/releases), then make it executable on Linux/macOS: `chmod +x steam-monitor-*`.
2. Copy `config.example.json` to `config.json`, and set `steam_api_key`. Obtain a key at <https://steamcommunity.com/dev/apikey>.
3. Start it: `./steam-monitor-linux-amd64 -config config.json` (use the downloaded filename on your platform).
4. Open <http://127.0.0.1:8080>, add public profiles, and use **Poll now** to initialise them. The add form accepts SteamID64, a Steam profile URL, a vanity URL, an `s.team/p/` link, or an 8–10 digit account ID; comma-separate multiple entries.

`config.json` controls the listen address, database location, polling interval, optional HTTP proxy, request timeout, and `retention_days`. The database and all recorded sessions are stored locally at `database_path`.

## Build from source

Requires Go 1.26 or newer:

```sh
go build -o steam-monitor .
cp config.example.json config.json
./steam-monitor
```

The WebUI source is in `frontend/` and uses npm, Vite, React, and TypeScript. To rebuild the embedded assets after changing the frontend:

```sh
cd frontend
npm install
npm run build
cd ..
go build -o steam-monitor .
```

The production build is written to `web/dist/` and embedded into the Go binary.

The WebUI includes dashboard status, player management with name/ID/game search and editable local nicknames, a proportional session timeline, team and per-player calendar heatmaps, activity rankings, daily activity bars, runtime diagnostics, and CSV/JSON session exports. Sessions are closed when a previously tracked player disappears from a successful poll response, and old records are removed according to `retention_days`.

The standalone scope intentionally excludes AstrBot collections, QQ/group bindings, chat commands, push notifications, and permission management.
