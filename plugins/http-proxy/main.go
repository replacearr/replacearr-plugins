// Package main is a Replacearr network_proxy plugin that advertises an HTTP,
// HTTPS, or SOCKS5 proxy URL for outbound indexer and metadata traffic.
//
// Build:
//
//	CGO_ENABLED=0 go build -o http-proxy .
package main

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/replacearr/replacearr/pkg/pluginsdk"
)

type HTTPProxyPlugin struct {
	mu       sync.RWMutex
	enabled  bool
	scheme   string
	hostname string
	port     int
	username string
	password string
}

func (p *HTTPProxyPlugin) Manifest() pluginsdk.Manifest {
	return pluginsdk.Manifest{
		ID:                "http-proxy",
		Name:              "HTTP / HTTPS / SOCKS Proxy",
		Version:           "0.1.0",
		Author:            "Replacearr",
		Description:       "Routes outbound indexer and metadata HTTP through a standard HTTP, HTTPS, or SOCKS5 proxy.",
		Types:             []pluginsdk.PluginType{pluginsdk.PluginTypeNetworkProxy},
		DisabledByDefault: true,
		ConfigSchema: []pluginsdk.ConfigFieldSchema{
			{Name: "enabled", Label: "Enabled", Type: "checkbox", Default: false, Required: false},
			{Name: "type", Label: "Proxy Type", Type: "select", Default: "http", Required: false, HelpText: "http, https, or socks5"},
			{Name: "hostname", Label: "Hostname", Type: "text", Default: "127.0.0.1", Required: false},
			{Name: "port", Label: "Port", Type: "number", Default: 8080, Required: false},
			{Name: "username", Label: "Username", Type: "text", Default: "", Required: false},
			{Name: "password", Label: "Password", Type: "password", Default: "", Required: false},
		},
	}
}

func (p *HTTPProxyPlugin) Configure(settings map[string]any) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.enabled = boolSetting(settings, "enabled", false)
	p.scheme = stringSetting(settings, "type", "http")
	if p.scheme == "" {
		p.scheme = "http"
	}
	p.hostname = stringSetting(settings, "hostname", "127.0.0.1")
	p.port = intSetting(settings, "port", 8080)
	p.username = stringSetting(settings, "username", "")
	p.password = stringSetting(settings, "password", "")
	return nil
}

func (p *HTTPProxyPlugin) ProxyURL() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.enabled || p.hostname == "" {
		return ""
	}
	hostPort := net.JoinHostPort(p.hostname, strconv.Itoa(p.port))
	conn, err := net.DialTimeout("tcp", hostPort, 400*time.Millisecond)
	if err != nil {
		return ""
	}
	_ = conn.Close()

	auth := ""
	if p.username != "" {
		if p.password != "" {
			auth = url.UserPassword(p.username, p.password).String() + "@"
		} else {
			auth = url.User(p.username).String() + "@"
		}
	}
	return fmt.Sprintf("%s://%s%s", p.scheme, auth, hostPort)
}

func boolSetting(settings map[string]any, key string, fallback bool) bool {
	if settings == nil {
		return fallback
	}
	v, ok := settings[key]
	if !ok {
		return fallback
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true") || t == "1"
	case float64:
		return t != 0
	default:
		return fallback
	}
}

func stringSetting(settings map[string]any, key, fallback string) string {
	if settings == nil {
		return fallback
	}
	if v, ok := settings[key].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

func intSetting(settings map[string]any, key string, fallback int) int {
	if settings == nil {
		return fallback
	}
	switch v := settings[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return fallback
}

func main() {
	pluginsdk.Serve(&HTTPProxyPlugin{})
}
