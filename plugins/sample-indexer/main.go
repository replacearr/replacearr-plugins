// Package main is a reference Replacearr Go plugin implementing a Torznab-style
// indexer in a single file with zero CGO dependencies.
//
// This demo invents fake releases for SDK learning.
//
// Build:
//
//	CGO_ENABLED=0 go build -o sample-indexer .
//
// Inspect manifest:
//
//	./sample-indexer --manifest
package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/replacearr/replacearr/pkg/pluginsdk"
)

// SampleIndexerPlugin implements pluginsdk.Plugin and pluginsdk.IndexerPlugin.
type SampleIndexerPlugin struct {
	mu            sync.RWMutex
	baseURL       string
	apiKey        string
	freeleechOnly bool
}

func (p *SampleIndexerPlugin) Manifest() pluginsdk.Manifest {
	return pluginsdk.Manifest{
		ID:          "sample-torznab-indexer",
		Name:        "Replacearr Torznab Demo Indexer",
		Version:     "0.1.0",
		Author:      "Replacearr",
		Description: "Reference Go indexer plugin demonstrating the zero-CGO Replacearr Plugin SDK over stdin/stdout JSON-RPC.",
		Types: []pluginsdk.PluginType{
			pluginsdk.PluginTypeIndexer,
		},
		DisabledByDefault: true,
		ConfigSchema: []pluginsdk.ConfigFieldSchema{
			{
				Name:     "base_url",
				Label:    "Tracker Base URL",
				Type:     "text",
				Default:  "https://demo-tracker.replacearr.local",
				Required: true,
				HelpText: "Base URL of the Torznab or custom tracker endpoint.",
			},
			{
				Name:     "api_key",
				Label:    "API Key",
				Type:     "password",
				Default:  "demo-api-key",
				Required: false,
				HelpText: "Optional API key for authenticated tracker queries.",
			},
			{
				Name:     "freeleech_only",
				Label:    "Freeleech Only",
				Type:     "checkbox",
				Default:  false,
				Required: false,
				HelpText: "Only return freeleech releases with high seeder counts.",
			},
		},
	}
}

func (p *SampleIndexerPlugin) Configure(settings map[string]any) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.baseURL = "https://demo-tracker.replacearr.local"
	if v, ok := settings["base_url"].(string); ok && strings.TrimSpace(v) != "" {
		p.baseURL = strings.TrimRight(strings.TrimSpace(v), "/")
	}
	if v, ok := settings["api_key"].(string); ok {
		p.apiKey = strings.TrimSpace(v)
	}
	if v, ok := settings["freeleech_only"].(bool); ok {
		p.freeleechOnly = v
	}
	return nil
}

func (p *SampleIndexerPlugin) Search(_ context.Context, req pluginsdk.SearchRequest) ([]pluginsdk.Release, error) {
	p.mu.RLock()
	baseURL := p.baseURL
	freeleechOnly := p.freeleechOnly
	p.mu.RUnlock()

	if baseURL == "" {
		baseURL = "https://demo-tracker.replacearr.local"
	}

	query := strings.TrimSpace(req.Query)
	if query == "" {
		query = "Replacearr.Sample.Media"
	}

	cleanTitle := strings.Join(strings.Fields(query), ".")
	if req.Season > 0 && req.Episode > 0 {
		cleanTitle = fmt.Sprintf("%s.S%02dE%02d", cleanTitle, req.Season, req.Episode)
	} else if req.Season > 0 {
		cleanTitle = fmt.Sprintf("%s.S%02d", cleanTitle, req.Season)
	}

	cats := req.Categories
	if len(cats) == 0 {
		if req.Type == "movie" {
			cats = []int{2000, 2040, 2045}
		} else {
			cats = []int{5000, 5040, 5045}
		}
	}

	now := time.Now().UTC()
	slug := strings.ToLower(strings.ReplaceAll(cleanTitle, ".", "-"))

	releases := []pluginsdk.Release{
		{
			Guid:        fmt.Sprintf("sample-torznab-%s-2160p", slug),
			Title:       fmt.Sprintf("%s.2160p.WEB-DL.DDP5.1.Atmos.H.265-REPLACEARR", cleanTitle),
			Protocol:    "torrent",
			Size:        8589934592, // 8.0 GiB
			Seeders:     148,
			Peers:       162,
			PublishDate: now.Add(-2 * time.Hour).Format(time.RFC3339),
			DownloadURL: fmt.Sprintf("%s/download/%s-2160p.torrent", baseURL, slug),
			InfoURL:     fmt.Sprintf("%s/details/%s-2160p", baseURL, slug),
			Categories:  cats,
			Quality:     "WEBDL-2160p",
		},
		{
			Guid:        fmt.Sprintf("sample-torznab-%s-1080p-bluray", slug),
			Title:       fmt.Sprintf("%s.1080p.BluRay.x264-REPLACEARR", cleanTitle),
			Protocol:    "torrent",
			Size:        4831838208, // 4.5 GiB
			Seeders:     215,
			Peers:       240,
			PublishDate: now.Add(-6 * time.Hour).Format(time.RFC3339),
			DownloadURL: fmt.Sprintf("%s/download/%s-1080p-bluray.torrent", baseURL, slug),
			InfoURL:     fmt.Sprintf("%s/details/%s-1080p-bluray", baseURL, slug),
			Categories:  cats,
			Quality:     "Bluray-1080p",
		},
		{
			Guid:        fmt.Sprintf("sample-torznab-%s-1080p-webdl", slug),
			Title:       fmt.Sprintf("%s.1080p.WEB-DL.DDP5.1.H.264-REPLACEARR", cleanTitle),
			Protocol:    "torrent",
			Size:        2684354560, // 2.5 GiB
			Seeders:     96,
			Peers:       110,
			PublishDate: now.Add(-12 * time.Hour).Format(time.RFC3339),
			DownloadURL: fmt.Sprintf("%s/download/%s-1080p-webdl.torrent", baseURL, slug),
			InfoURL:     fmt.Sprintf("%s/details/%s-1080p-webdl", baseURL, slug),
			Categories:  cats,
			Quality:     "WEBDL-1080p",
		},
	}

	if freeleechOnly {
		releases = releases[:2]
	}
	if req.Limit > 0 && len(releases) > req.Limit {
		releases = releases[:req.Limit]
	}

	return releases, nil
}

func main() {
	pluginsdk.Serve(&SampleIndexerPlugin{})
}
