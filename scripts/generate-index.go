// Command generate-index walks plugins/**/plugin.json and writes index.json.
//
// Usage:
//
//	go run ./scripts/generate-index.go [-assets .artifacts/checksums.json] [-out index.json]
//
// When -assets points at a JSON map of "pluginID/goos_goarch" -> {url,sha256,size},
// those entries are merged into each plugin's assets array.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const schemaVersion = 1

type pluginMeta struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Version               string   `json:"version"`
	Author                string   `json:"author"`
	Description           string   `json:"description"`
	Types                 []string `json:"types"`
	Homepage              string   `json:"homepage"`
	Repository            string   `json:"repository"`
	MinReplacearrVersion  string   `json:"minReplacearrVersion"`
	Entry                 string   `json:"entry"`
	Module                string   `json:"module"`
}

type asset struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type catalogPlugin struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Version              string   `json:"version"`
	Author               string   `json:"author"`
	Description          string   `json:"description"`
	Types                []string `json:"types"`
	Homepage             string   `json:"homepage"`
	Repository           string   `json:"repository"`
	MinReplacearrVersion string   `json:"minReplacearrVersion"`
	Assets               []asset  `json:"assets"`
}

type catalog struct {
	SchemaVersion int             `json:"schemaVersion"`
	UpdatedAt     string          `json:"updatedAt"`
	Plugins       []catalogPlugin `json:"plugins"`
}

type assetInfo struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	GOOS   string `json:"goos,omitempty"`
	GOARCH string `json:"goarch,omitempty"`
}

func main() {
	root := flag.String("root", ".", "repository root")
	outPath := flag.String("out", "index.json", "output path for index.json")
	assetsPath := flag.String("assets", "", "optional JSON map of assets keyed by pluginID/goos_goarch")
	flag.Parse()

	pluginsRoot := filepath.Join(*root, "plugins")
	metas, err := loadPlugins(pluginsRoot)
	if err != nil {
		fail(err)
	}

	assetMap := map[string]assetInfo{}
	if *assetsPath != "" {
		raw, err := os.ReadFile(*assetsPath)
		if err != nil {
			fail(err)
		}
		if err := json.Unmarshal(raw, &assetMap); err != nil {
			fail(fmt.Errorf("parse assets: %w", err))
		}
	}

	// Preserve prior assets for plugins not present in this -assets map (single-plugin releases).
	prevAssets := map[string][]asset{}
	out := filepath.Join(*root, *outPath)
	if filepath.IsAbs(*outPath) {
		out = *outPath
	}
	if raw, err := os.ReadFile(out); err == nil {
		var prev catalog
		if json.Unmarshal(raw, &prev) == nil {
			for _, p := range prev.Plugins {
				prevAssets[p.ID] = p.Assets
			}
		}
	}

	cat := catalog{
		SchemaVersion: schemaVersion,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		Plugins:       make([]catalogPlugin, 0, len(metas)),
	}

	for _, meta := range metas {
		entry := catalogPlugin{
			ID:                   meta.ID,
			Name:                 meta.Name,
			Version:              meta.Version,
			Author:               meta.Author,
			Description:          meta.Description,
			Types:                meta.Types,
			Homepage:             meta.Homepage,
			Repository:           meta.Repository,
			MinReplacearrVersion: meta.MinReplacearrVersion,
			Assets:               []asset{},
		}
		prefix := meta.ID + "/"
		for key, info := range assetMap {
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			rest := strings.TrimPrefix(key, prefix)
			goos, goarch, ok := splitPlatform(rest, info)
			if !ok {
				continue
			}
			if info.SHA256 == "" || info.URL == "" {
				fail(fmt.Errorf("asset %s missing url or sha256", key))
			}
			entry.Assets = append(entry.Assets, asset{
				GOOS:   goos,
				GOARCH: goarch,
				URL:    info.URL,
				SHA256: info.SHA256,
				Size:   info.Size,
			})
		}
		if len(entry.Assets) == 0 {
			if prior, ok := prevAssets[meta.ID]; ok {
				entry.Assets = append([]asset{}, prior...)
			}
		}
		sort.Slice(entry.Assets, func(i, j int) bool {
			if entry.Assets[i].GOOS != entry.Assets[j].GOOS {
				return entry.Assets[i].GOOS < entry.Assets[j].GOOS
			}
			return entry.Assets[i].GOARCH < entry.Assets[j].GOARCH
		})
		cat.Plugins = append(cat.Plugins, entry)
	}

	sort.Slice(cat.Plugins, func(i, j int) bool {
		return cat.Plugins[i].ID < cat.Plugins[j].ID
	})

	encoded, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		fail(err)
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(out, encoded, 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("wrote %s (%d plugins)\n", out, len(cat.Plugins))
}

func loadPlugins(root string) ([]pluginMeta, error) {
	var metas []pluginMeta
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != "plugin.json" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var meta pluginMeta
		if err := json.Unmarshal(raw, &meta); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if meta.ID == "" || meta.Name == "" || meta.Version == "" {
			return fmt.Errorf("%s: id, name, and version are required", path)
		}
		metas = append(metas, meta)
		return nil
	})
	return metas, err
}

func splitPlatform(rest string, info assetInfo) (goos, goarch string, ok bool) {
	if info.GOOS != "" && info.GOARCH != "" {
		return info.GOOS, info.GOARCH, true
	}
	parts := strings.Split(rest, "_")
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "generate-index: %v\n", err)
	os.Exit(1)
}
