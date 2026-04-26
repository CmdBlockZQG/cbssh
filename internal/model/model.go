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

type Config struct {
	DefaultKeyPath string `toml:"default_key_path"`
	HostKeyCheck   string `toml:"host_key_check"`
	Hosts          []Host `toml:"hosts"`
}

type Host struct {
	Name    string   `toml:"name"`
	Host    string   `toml:"host"`
	Port    int      `toml:"port"`
	User    string   `toml:"user"`
	Jump    string   `toml:"jump,omitempty"`
	Auth    Auth     `toml:"auth"`
	Tunnels []Tunnel `toml:"tunnels,omitempty"`
}

type Auth struct {
	Type       string `toml:"type"`
	Password   string `toml:"password,omitempty"`
	KeyPath    string `toml:"key_path,omitempty"`
	Passphrase string `toml:"passphrase,omitempty"`
	UseAgent   bool   `toml:"use_agent,omitempty"`
}

type Tunnel struct {
	Name       string `toml:"name"`
	Type       string `toml:"type"`
	ListenHost string `toml:"listen_host"`
	ListenPort int    `toml:"listen_port"`
	TargetHost string `toml:"target_host,omitempty"`
	TargetPort int    `toml:"target_port,omitempty"`
	Default    bool   `toml:"default"`
}

type State struct {
	Version int                    `json:"version"`
	Hosts   map[string]HostRuntime `json:"hosts,omitempty"`
	Tunnels []TunnelRuntime        `json:"tunnels,omitempty"`
}

type HostRuntime struct {
	LastUsed time.Time `json:"last_used,omitempty"`
}

type TunnelRuntime struct {
	ID         string    `json:"id"`
	RunID      string    `json:"run_id"`
	HostName   string    `json:"host_name"`
	TunnelName string    `json:"tunnel_name"`
	Type       string    `json:"type"`
	PID        int       `json:"pid"`
	ListenHost string    `json:"listen_host"`
	ListenPort int       `json:"listen_port"`
	TargetHost string    `json:"target_host,omitempty"`
	TargetPort int       `json:"target_port,omitempty"`
	JumpChain  []string  `json:"jump_chain,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	LogPath    string    `json:"log_path"`
}

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

func (h Host) Address() string {
	return net.JoinHostPort(h.Host, strconv.Itoa(h.Port))
}

func (t Tunnel) ListenAddress() string {
	return net.JoinHostPort(t.ListenHost, strconv.Itoa(t.ListenPort))
}

func (t Tunnel) TargetAddress() string {
	return net.JoinHostPort(t.TargetHost, strconv.Itoa(t.TargetPort))
}

func (t TunnelRuntime) ListenAddress() string {
	return net.JoinHostPort(t.ListenHost, strconv.Itoa(t.ListenPort))
}

func (t TunnelRuntime) TargetAddress() string {
	if t.TargetHost == "" || t.TargetPort == 0 {
		return ""
	}
	return net.JoinHostPort(t.TargetHost, strconv.Itoa(t.TargetPort))
}

func (t TunnelRuntime) Key() string {
	return RuntimeKey(t.HostName, t.TunnelName)
}

func RuntimeKey(hostName, tunnelName string) string {
	return fmt.Sprintf("%s/%s", hostName, tunnelName)
}
