// Package steam wraps the Steam Web API and converts user-friendly profile inputs.
package steam

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"steam-monitor/store"
)

const steamIDBase int64 = 76561197960265728

type Client struct {
	key, base string
	http      *http.Client
}
type response struct {
	Response struct {
		Players []struct {
			SteamID string `json:"steamid"`
			Name    string `json:"personaname"`
			Avatar  string `json:"avatarfull"`
			State   int    `json:"personastate"`
			GameID  string `json:"gameid"`
			Game    string `json:"gameextrainfo"`
		} `json:"players"`
	} `json:"response"`
}

func New(key, base, proxy string, timeout time.Duration) (*Client, error) {
	transport := &http.Transport{}
	if proxy != "" {
		u, err := url.Parse(proxy)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy_url: %w", err)
		}
		transport.Proxy = http.ProxyURL(u)
	}
	return &Client{key: key, base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: timeout, Transport: transport}}, nil
}
func (c *Client) Players(ctx context.Context, ids []string) ([]store.Player, error) {
	var out []store.Player
	for start := 0; start < len(ids); start += 100 {
		end := start + 100
		if end > len(ids) {
			end = len(ids)
		}
		endpoint := c.base + "/ISteamUser/GetPlayerSummaries/v0002/?key=" + url.QueryEscape(c.key) + "&steamids=" + url.QueryEscape(strings.Join(ids[start:end], ","))
		var parsed response
		if err := c.getJSON(ctx, endpoint, &parsed); err != nil {
			return nil, err
		}
		for _, p := range parsed.Response.Players {
			gid, _ := strconv.ParseInt(p.GameID, 10, 64)
			out = append(out, store.Player{SteamID: p.SteamID, Name: p.Name, Avatar: p.Avatar, State: p.State, GameID: gid, Game: p.Game})
		}
	}
	return out, nil
}

// Resolve accepts the formats accepted by the original plugin: SteamID64, profile
// links, vanity links, s.team links that contain an ID, and an 8-10 digit account ID.
func (c *Client) Resolve(ctx context.Context, raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if id := steamID64(s); id != "" {
		log.Printf("Steam input resolved: source=steamid64 steam_id=%s", id)
		return id, nil
	}
	parsed, err := url.Parse(s)
	if err == nil && parsed.Host != "" {
		host := strings.ToLower(parsed.Hostname())
		segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if host == "s.team" || strings.HasSuffix(host, ".s.team") {
			if len(segments) > 0 && steamID64(segments[len(segments)-1]) != "" {
				log.Printf("Steam input resolved: source=s.team-direct steam_id=%s", segments[len(segments)-1])
				return segments[len(segments)-1], nil
			}
			return c.resolveShortURL(ctx, parsed.String())
		}
		if host == "steamcommunity.com" || strings.HasSuffix(host, ".steamcommunity.com") {
			if len(segments) >= 2 && strings.EqualFold(segments[0], "profiles") && steamID64(segments[1]) != "" {
				log.Printf("Steam input resolved: source=profile-url steam_id=%s", segments[1])
				return segments[1], nil
			}
			if len(segments) >= 2 && strings.EqualFold(segments[0], "id") && segments[1] != "" {
				return c.resolveVanity(ctx, segments[1])
			}
		}
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 && len(s) <= 10 {
		id := strconv.FormatInt(steamIDBase+n, 10)
		log.Printf("Steam input resolved: source=account-id steam_id=%s", id)
		return id, nil
	}
	return "", fmt.Errorf("unsupported Steam ID input %q", raw)
}

func (c *Client) resolveShortURL(ctx context.Context, raw string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return "", err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("resolve s.team link: %w", err)
	}
	res.Body.Close()
	if res.Request == nil || res.Request.URL == nil || res.Request.URL.String() == raw {
		return "", fmt.Errorf("Steam could not resolve short link %q", raw)
	}
	return c.Resolve(ctx, res.Request.URL.String())
}
func steamID64(s string) string {
	if len(s) != 17 {
		return ""
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return s
}
func (c *Client) resolveVanity(ctx context.Context, vanity string) (string, error) {
	var reply struct {
		Response struct {
			Success int    `json:"success"`
			SteamID string `json:"steamid"`
		} `json:"response"`
	}
	endpoint := c.base + "/ISteamUser/ResolveVanityURL/v1/?key=" + url.QueryEscape(c.key) + "&vanityurl=" + url.QueryEscape(vanity)
	if err := c.getJSON(ctx, endpoint, &reply); err != nil {
		return "", err
	}
	if reply.Response.Success != 1 || steamID64(reply.Response.SteamID) == "" {
		return "", fmt.Errorf("Steam could not resolve vanity ID %q", vanity)
	}
	log.Printf("Steam input resolved: source=vanity-url vanity=%q steam_id=%s", vanity, reply.Response.SteamID)
	return reply.Response.SteamID, nil
}
func (c *Client) getJSON(ctx context.Context, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("Steam API returned %s", res.Status)
	}
	return json.Unmarshal(body, target)
}
