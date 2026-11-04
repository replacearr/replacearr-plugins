// Package main is a Replacearr notification plugin that POSTs events to a webhook URL.
//
// Build:
//
//	CGO_ENABLED=0 go build -o webhook-notifier .
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/replacearr/replacearr/pkg/pluginsdk"
)

type WebhookNotifier struct {
	mu         sync.RWMutex
	url        string
	secret     string
	onStarted  bool
	onComplete bool
	onStalled  bool
	onMoved    bool
	onJellyfin bool
	client     *http.Client
}

func (p *WebhookNotifier) Manifest() pluginsdk.Manifest {
	return pluginsdk.Manifest{
		ID:                "webhook-notifier",
		Name:              "Webhook Notifier",
		Version:           "0.1.0",
		Author:            "SiloCityLabs",
		Description:       "POSTs Replacearr lifecycle events as JSON to a webhook URL. Use as a template for Telegram, Signal, and other channel plugins.",
		Types:             []pluginsdk.PluginType{pluginsdk.PluginTypeNotification},
		DisabledByDefault: true,
		ConfigSchema: []pluginsdk.ConfigFieldSchema{
			{
				Name:     "webhook_url",
				Label:    "Webhook URL",
				Type:     "text",
				Default:  "",
				Required: true,
				HelpText: "HTTPS endpoint that accepts POST JSON payloads.",
			},
			{
				Name:     "shared_secret",
				Label:    "Shared Secret Header",
				Type:     "password",
				Default:  "",
				Required: false,
				HelpText: "Optional value sent as X-Replacearr-Secret.",
			},
			{Name: "onDownloadStarted", Label: "Download started", Type: "checkbox", Default: true, Required: false},
			{Name: "onDownloadComplete", Label: "Download complete", Type: "checkbox", Default: true, Required: false},
			{Name: "onDownloadStalled", Label: "Downloading more than X days", Type: "checkbox", Default: true, Required: false},
			{Name: "onDownloadMoved", Label: "Download moved", Type: "checkbox", Default: true, Required: false},
			{Name: "onJellyfinPickedUp", Label: "Jellyfin picked up", Type: "checkbox", Default: true, Required: false},
		},
	}
}

func (p *WebhookNotifier) Configure(settings map[string]any) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.url = ""
	if v, ok := settings["webhook_url"].(string); ok {
		p.url = strings.TrimSpace(v)
	}
	p.secret = ""
	if v, ok := settings["shared_secret"].(string); ok {
		p.secret = strings.TrimSpace(v)
	}
	p.onStarted = boolSetting(settings, "onDownloadStarted", true)
	p.onComplete = boolSetting(settings, "onDownloadComplete", true)
	p.onStalled = boolSetting(settings, "onDownloadStalled", true)
	p.onMoved = boolSetting(settings, "onDownloadMoved", true)
	p.onJellyfin = boolSetting(settings, "onJellyfinPickedUp", true)
	if p.client == nil {
		p.client = &http.Client{Timeout: 15 * time.Second}
	}
	return nil
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

func (p *WebhookNotifier) eventEnabled(eventType string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	switch eventType {
	case pluginsdk.EventNotificationTest:
		return true
	case pluginsdk.EventDownloadStarted:
		return p.onStarted
	case pluginsdk.EventDownloadComplete:
		return p.onComplete
	case pluginsdk.EventDownloadStalled:
		return p.onStalled
	case pluginsdk.EventDownloadMoved:
		return p.onMoved
	case pluginsdk.EventJellyfinPickedUp:
		return p.onJellyfin
	default:
		return false
	}
}

func (p *WebhookNotifier) Notify(ctx context.Context, eventType string, payload map[string]any) error {
	if !p.eventEnabled(eventType) {
		return nil
	}
	p.mu.RLock()
	url := p.url
	secret := p.secret
	client := p.client
	p.mu.RUnlock()

	if url == "" {
		return fmt.Errorf("webhook_url is not configured")
	}
	if payload == nil {
		payload = map[string]any{}
	}
	body, err := json.Marshal(map[string]any{
		"eventType": eventType,
		"payload":   payload,
		"sentAt":    time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "replacearr-webhook-notifier/0.1")
	if secret != "" {
		req.Header.Set("X-Replacearr-Secret", secret)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func main() {
	pluginsdk.Serve(&WebhookNotifier{})
}
