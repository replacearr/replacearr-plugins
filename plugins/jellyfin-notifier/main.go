// Package main is a Replacearr notification plugin that refreshes a Jellyfin library
// after imports, and handles inbound Jellyfin webhook callbacks via the host.
//
// Build:
//
//	CGO_ENABLED=0 go build -o jellyfin-notifier .
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/replacearr/replacearr/pkg/pluginsdk"
)

type JellyfinNotifier struct {
	mu         sync.RWMutex
	baseURL    string
	apiKey     string
	mode       string
	pathFrom   string
	pathTo     string
	onMoved    bool
	onComplete bool
	client     *http.Client
}

func (p *JellyfinNotifier) Manifest() pluginsdk.Manifest {
	return pluginsdk.Manifest{
		ID:          "jellyfin-notifier",
		Name:        "Jellyfin Notifier",
		Version:     "0.1.1",
		Author:      "Replacearr",
		Description: "Refreshes Jellyfin after Replacearr imports media, and turns Jellyfin Webhook ItemAdded callbacks into jellyfin.picked_up events.",
		Types:       []pluginsdk.PluginType{pluginsdk.PluginTypeNotification},
		DisabledByDefault: true,
		InboundWebhooks: []pluginsdk.InboundWebhookSpec{{
			SecretSetting: "inbound_webhook_secret",
			PathAliases:   []string{"jellyfin"},
			Description:   "Jellyfin Webhook plugin ItemAdded → POST /api/v1/webhooks/jellyfin",
		}},
		ConfigSchema: []pluginsdk.ConfigFieldSchema{
			{
				Name:     "base_url",
				Label:    "Jellyfin Base URL",
				Type:     "text",
				Default:  "",
				Required: true,
				HelpText: "e.g. http://jellyfin:8096 (no trailing slash).",
			},
			{
				Name:     "api_key",
				Label:    "API Key",
				Type:     "password",
				Default:  "",
				Required: true,
				HelpText: "Jellyfin API key sent as X-Emby-Token.",
			},
			{
				Name:     "refresh_mode",
				Label:    "Refresh Mode",
				Type:     "select",
				Default:  "path",
				Required: false,
				HelpText: "path = POST /Library/Media/Updated for the imported file; full = POST /Library/Refresh.",
			},
			{
				Name:     "path_from",
				Label:    "Path Prefix From",
				Type:     "text",
				Default:  "",
				Required: false,
				HelpText: "Replacearr-side path prefix to rewrite (e.g. /data/Movies).",
			},
			{
				Name:     "path_to",
				Label:    "Path Prefix To",
				Type:     "text",
				Default:  "",
				Required: false,
				HelpText: "Jellyfin-side path prefix (e.g. /media/movies).",
			},
			{Name: "onDownloadMoved", Label: "Download moved", Type: "checkbox", Default: true, Required: false},
			{Name: "onDownloadComplete", Label: "Download complete", Type: "checkbox", Default: false, Required: false},
			{
				Name:     "inbound_webhook_secret",
				Label:    "Inbound Webhook Secret",
				Type:     "password",
				Default:  "",
				Required: false,
				HelpText: "Shared secret for POST /api/v1/webhooks/jellyfin (header X-Replacearr-Webhook-Secret or ?secret=).",
			},
		},
	}
}

func (p *JellyfinNotifier) Configure(settings map[string]any) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.baseURL = strings.TrimRight(strings.TrimSpace(stringSetting(settings, "base_url")), "/")
	p.apiKey = strings.TrimSpace(stringSetting(settings, "api_key"))
	p.mode = strings.ToLower(strings.TrimSpace(stringSetting(settings, "refresh_mode")))
	if p.mode == "" {
		p.mode = "path"
	}
	p.pathFrom = stringSetting(settings, "path_from")
	p.pathTo = stringSetting(settings, "path_to")
	p.onMoved = boolSetting(settings, "onDownloadMoved", true)
	p.onComplete = boolSetting(settings, "onDownloadComplete", false)
	if p.client == nil {
		p.client = &http.Client{Timeout: 20 * time.Second}
	}
	return nil
}

func stringSetting(settings map[string]any, key string) string {
	v, ok := settings[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func boolSetting(settings map[string]any, key string, fallback bool) bool {
	v, ok := settings[key]
	if !ok {
		return fallback
	}
	b, ok := v.(bool)
	if !ok {
		return fallback
	}
	return b
}

func (p *JellyfinNotifier) eventEnabled(eventType string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	switch eventType {
	case pluginsdk.EventNotificationTest:
		return true
	case pluginsdk.EventDownloadMoved:
		return p.onMoved
	case pluginsdk.EventDownloadComplete:
		return p.onComplete
	default:
		return false
	}
}

func (p *JellyfinNotifier) Notify(ctx context.Context, eventType string, payload map[string]any) error {
	if !p.eventEnabled(eventType) {
		return nil
	}
	p.mu.RLock()
	baseURL := p.baseURL
	apiKey := p.apiKey
	mode := p.mode
	pathFrom := p.pathFrom
	pathTo := p.pathTo
	client := p.client
	p.mu.RUnlock()

	if baseURL == "" {
		return fmt.Errorf("base_url is not configured")
	}
	if apiKey == "" {
		return fmt.Errorf("api_key is not configured")
	}

	if eventType == pluginsdk.EventNotificationTest {
		return p.getSystemInfo(ctx, client, baseURL, apiKey)
	}

	path := mediaPathFromPayload(payload)
	path = rewritePath(path, pathFrom, pathTo)
	if mode == "full" || path == "" {
		return p.postRefresh(ctx, client, baseURL, apiKey, "/Library/Refresh", nil)
	}
	body, err := json.Marshal(map[string]any{
		"Updates": []map[string]string{
			{"Path": path, "UpdateType": "Created"},
		},
	})
	if err != nil {
		return err
	}
	return p.postRefresh(ctx, client, baseURL, apiKey, "/Library/Media/Updated", body)
}

// HandleWebhook parses Jellyfin Webhook plugin JSON and emits jellyfin.picked_up for ItemAdded.
func (p *JellyfinNotifier) HandleWebhook(_ context.Context, req pluginsdk.WebhookRequest) (*pluginsdk.WebhookResult, error) {
	raw := map[string]any{}
	if len(bytes.TrimSpace(req.Body)) > 0 {
		if err := json.Unmarshal(req.Body, &raw); err != nil {
			return &pluginsdk.WebhookResult{Status: http.StatusBadRequest}, fmt.Errorf("invalid json: %w", err)
		}
	}
	if !jellyfinWebhookIsItemAdded(raw) {
		return &pluginsdk.WebhookResult{Status: http.StatusNoContent}, nil
	}
	return &pluginsdk.WebhookResult{
		Status: http.StatusNoContent,
		Events: []pluginsdk.WebhookEvent{{
			EventType: pluginsdk.EventJellyfinPickedUp,
			Payload:   normalizeJellyfinWebhookPayload(raw),
		}},
	}, nil
}

func jellyfinWebhookIsItemAdded(raw map[string]any) bool {
	notifType := firstString(raw,
		"NotificationType", "notificationType", "notification_type",
		"Event", "event", "Type", "type",
	)
	if notifType == "" {
		return true
	}
	n := strings.ToLower(strings.TrimSpace(notifType))
	switch n {
	case "itemadded", "item.added", "library.new", "library_new", "item added":
		return true
	}
	if strings.Contains(n, "playback") || strings.Contains(n, "session") ||
		strings.Contains(n, "userdatal") || strings.Contains(n, "authentication") {
		return false
	}
	return false
}

func normalizeJellyfinWebhookPayload(raw map[string]any) map[string]any {
	out := map[string]any{}
	if title := firstString(raw, "Name", "ItemName", "title", "SeriesName"); title != "" {
		out["title"] = title
	}
	if itemType := firstString(raw, "ItemType", "item_type", "itemType"); itemType != "" {
		out["itemType"] = itemType
	}
	if itemID := firstString(raw, "ItemId", "Id", "itemId", "item_id"); itemID != "" {
		out["itemId"] = itemID
	}
	if path := firstString(raw, "Path", "ItemPath", "path", "item_path"); path != "" {
		out["path"] = path
	}
	return out
}

func firstString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := raw[key]; ok {
			if s, ok := v.(string); ok {
				if t := strings.TrimSpace(s); t != "" {
					return t
				}
			}
		}
	}
	return ""
}

func mediaPathFromPayload(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	for _, key := range []string{"destPath", "path"} {
		if v, ok := payload[key].(string); ok {
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func rewritePath(path, from, to string) string {
	path = strings.TrimSpace(path)
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if path == "" || from == "" {
		return path
	}
	if strings.HasPrefix(path, from) {
		return to + strings.TrimPrefix(path, from)
	}
	return path
}

func (p *JellyfinNotifier) getSystemInfo(ctx context.Context, client *http.Client, baseURL, apiKey string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/System/Info", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Emby-Token", apiKey)
	req.Header.Set("User-Agent", "replacearr-jellyfin-notifier/0.1")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("jellyfin System/Info returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (p *JellyfinNotifier) postRefresh(ctx context.Context, client *http.Client, baseURL, apiKey, apiPath string, body []byte) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+apiPath, reader)
	if err != nil {
		return err
	}
	req.Header.Set("X-Emby-Token", apiKey)
	req.Header.Set("User-Agent", "replacearr-jellyfin-notifier/0.1")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("jellyfin %s returned HTTP %d", apiPath, resp.StatusCode)
	}
	return nil
}

func main() {
	pluginsdk.Serve(&JellyfinNotifier{})
}
