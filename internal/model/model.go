package model

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	AuthTypeKey      = "key"
	AuthTypePassword = "password"

	TunnelTypeLocal   = "local"
	TunnelTypeRemote  = "remote"
	TunnelTypeDynamic = "dynamic"
)

func TunnelTypeCode(value string) string {
	switch value {
	case TunnelTypeDynamic:
		return "D"
	case TunnelTypeLocal:
		return "L"
	case TunnelTypeRemote:
		return "R"
	default:
		return value
	}
}

// Config is the top-level TOML configuration file.
type Config struct {
	DefaultKeyPath string `toml:"default_key_path"`
	HostKeyCheck   string `toml:"host_key_check"`
	Hosts          []Host `toml:"hosts"`
}

// Host is the persisted SSH endpoint definition, including optional tunnels.
type Host struct {
	Name    string   `toml:"name"`
	Host    string   `toml:"host"`
	Port    int      `toml:"port"`
	User    string   `toml:"user"`
	Jump    string   `toml:"jump,omitempty"`
	Auth    Auth     `toml:"auth"`
	Tunnels []Tunnel `toml:"tunnels,omitempty"`
}

// Auth stores the authentication material for a host.
type Auth struct {
	Type       string `toml:"type"`
	Password   string `toml:"password,omitempty"`
	KeyPath    string `toml:"key_path,omitempty"`
	Passphrase string `toml:"passphrase,omitempty"`
	UseAgent   bool   `toml:"use_agent,omitempty"`
}

// Tunnel describes one ssh -L, ssh -R, or ssh -D style forwarding rule.
type Tunnel struct {
	Name       string `toml:"name"`
	Type       string `toml:"type"`
	ListenHost string `toml:"listen_host"`
	ListenPort int    `toml:"listen_port"`
	TargetHost string `toml:"target_host,omitempty"`
	TargetPort int    `toml:"target_port,omitempty"`
	Default    bool   `toml:"default"`
}

// State stores user recency data and currently tracked tunnel daemons.
type State struct {
	Version int                    `json:"version"`
	Hosts   map[string]HostRuntime `json:"hosts,omitempty"`
	Tunnels []TunnelRuntime        `json:"tunnels,omitempty"`
}

// HostRuntime stores runtime-only host metadata.
type HostRuntime struct {
	LastUsed time.Time `json:"last_used,omitempty"`
}

// TunnelRuntime is the state entry written by the daemon after a listener starts.
type TunnelRuntime struct {
	ID          string    `json:"id"`
	RunID       string    `json:"run_id"`
	HostName    string    `json:"host_name"`
	TunnelName  string    `json:"tunnel_name"`
	Type        string    `json:"type"`
	PID         int       `json:"pid"`
	ProcessKey  string    `json:"process_key,omitempty"`
	ControlPath string    `json:"control_path,omitempty"`
	ListenHost  string    `json:"listen_host"`
	ListenPort  int       `json:"listen_port"`
	TargetHost  string    `json:"target_host,omitempty"`
	TargetPort  int       `json:"target_port,omitempty"`
	JumpChain   []string  `json:"jump_chain,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	LogPath     string    `json:"log_path"`
}

// Normalize fills backward-compatible defaults and trims user-controlled names.
func (c *Config) Normalize() {
	if c.DefaultKeyPath == "" {
		c.DefaultKeyPath = "~/.ssh/id_ed25519"
	}
	if c.HostKeyCheck == "" {
		c.HostKeyCheck = "insecure"
	}
	for i := range c.Hosts {
		c.Hosts[i].Normalize(c.DefaultKeyPath)
	}
}

// Normalize fills host defaults inherited from the global config.
func (h *Host) Normalize(defaultKeyPath string) {
	h.Name = strings.TrimSpace(h.Name)
	h.Host = strings.TrimSpace(h.Host)
	h.User = strings.TrimSpace(h.User)
	h.Jump = strings.TrimSpace(h.Jump)
	if h.Port == 0 {
		h.Port = 22
	}
	if h.Auth.Type == "" {
		if h.Auth.Password != "" {
			h.Auth.Type = AuthTypePassword
		} else {
			h.Auth.Type = AuthTypeKey
		}
	}
	if h.Auth.Type == AuthTypeKey && h.Auth.KeyPath == "" {
		h.Auth.KeyPath = defaultKeyPath
	}
	for i := range h.Tunnels {
		h.Tunnels[i].Normalize()
	}
}

// Normalize fills tunnel defaults that match OpenSSH's local bind behavior.
func (t *Tunnel) Normalize() {
	t.Name = strings.TrimSpace(t.Name)
	t.Type = strings.TrimSpace(t.Type)
	t.ListenHost = strings.TrimSpace(t.ListenHost)
	t.TargetHost = strings.TrimSpace(t.TargetHost)
	if t.Type == "" {
		t.Type = TunnelTypeLocal
	}
	if t.ListenHost == "" {
		t.ListenHost = "127.0.0.1"
	}
}

// Address returns the canonical host:port SSH endpoint.
func (h Host) Address() string {
	return net.JoinHostPort(h.Host, strconv.Itoa(h.Port))
}

// ListenAddress returns the local or remote listener endpoint for a tunnel.
func (t Tunnel) ListenAddress() string {
	return net.JoinHostPort(t.ListenHost, strconv.Itoa(t.ListenPort))
}

// TargetAddress returns the target endpoint, or empty for dynamic tunnels.
func (t Tunnel) TargetAddress() string {
	if t.TargetHost == "" || t.TargetPort == 0 {
		return ""
	}
	return net.JoinHostPort(t.TargetHost, strconv.Itoa(t.TargetPort))
}

// ListenAddress returns the listener endpoint from persisted runtime state.
func (t TunnelRuntime) ListenAddress() string {
	return net.JoinHostPort(t.ListenHost, strconv.Itoa(t.ListenPort))
}

// TargetAddress returns the target endpoint, or empty for dynamic tunnels.
func (t TunnelRuntime) TargetAddress() string {
	if t.TargetHost == "" || t.TargetPort == 0 {
		return ""
	}
	return net.JoinHostPort(t.TargetHost, strconv.Itoa(t.TargetPort))
}

// Key returns the stable host/tunnel identity shared across daemon runs.
func (t TunnelRuntime) Key() string {
	return RuntimeKey(t.HostName, t.TunnelName)
}

// RuntimeKey returns the stable key used to deduplicate active tunnel entries.
func RuntimeKey(hostName, tunnelName string) string {
	return fmt.Sprintf("%s/%s", hostName, tunnelName)
}
