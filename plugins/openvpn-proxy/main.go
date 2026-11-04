// Package main is a Replacearr network_proxy plugin that exposes a local SOCKS
// endpoint, optionally starting OpenVPN when a config and binary are available.
//
// Build:
//
//	CGO_ENABLED=0 go build -o openvpn-proxy .
package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/replacearr/replacearr/pkg/pluginsdk"
)

type OpenVPNProxyPlugin struct {
	mu          sync.RWMutex
	configPath  string
	socksListen string
	openvpnBin  string
	cmd         *exec.Cmd
}

func (p *OpenVPNProxyPlugin) Manifest() pluginsdk.Manifest {
	return pluginsdk.Manifest{
		ID:          "openvpn-proxy",
		Name:        "OpenVPN SOCKS Proxy",
		Version:     "0.1.0",
		Author:            "Replacearr",
		Description:       "Exposes a local SOCKS5 URL for outbound traffic; optionally launches OpenVPN (requires CAP_NET_ADMIN).",
		Types:             []pluginsdk.PluginType{pluginsdk.PluginTypeNetworkProxy},
		DisabledByDefault: true,
		ConfigSchema: []pluginsdk.ConfigFieldSchema{
			{
				Name:     "configPath",
				Label:    "OpenVPN Config Path",
				Type:     "text",
				Default:  "",
				Required: false,
				HelpText: "Path to an .ovpn file. Leave empty to only advertise the SOCKS listen URL.",
			},
			{
				Name:     "socksListen",
				Label:    "SOCKS Listen Address",
				Type:     "text",
				Default:  "127.0.0.1:1080",
				Required: true,
				HelpText: "Local address OpenVPN (or a companion SOCKS bridge) listens on.",
			},
			{
				Name:     "openvpnBinary",
				Label:    "OpenVPN Binary",
				Type:     "text",
				Default:  "openvpn",
				Required: false,
				HelpText: "Path or name of the openvpn binary. Startup is best-effort.",
			},
		},
	}
}

func (p *OpenVPNProxyPlugin) Configure(settings map[string]any) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.configPath = stringSetting(settings, "configPath", "")
	p.socksListen = stringSetting(settings, "socksListen", "127.0.0.1:1080")
	p.openvpnBin = stringSetting(settings, "openvpnBinary", "openvpn")
	if p.socksListen == "" {
		p.socksListen = "127.0.0.1:1080"
	}

	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		p.cmd = nil
	}

	if p.configPath == "" {
		return nil
	}
	bin := p.openvpnBin
	if bin == "" {
		bin = "openvpn"
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		// Best-effort: still expose ProxyURL even if openvpn is missing.
		return nil
	}
	cmd := exec.Command(path, "--config", p.configPath, "--daemon", "--writepid", "/tmp/replacearr-openvpn.pid")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start openvpn: %w", err)
	}
	p.cmd = cmd
	return nil
}

func (p *OpenVPNProxyPlugin) ProxyURL() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	listen := strings.TrimSpace(p.socksListen)
	if listen == "" {
		listen = "127.0.0.1:1080"
	}
	// Do not advertise a SOCKS URL unless something is actually listening.
	conn, err := net.DialTimeout("tcp", listen, 400*time.Millisecond)
	if err != nil {
		return ""
	}
	_ = conn.Close()
	return "socks5://" + listen
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

func main() {
	pluginsdk.Serve(&OpenVPNProxyPlugin{})
}
