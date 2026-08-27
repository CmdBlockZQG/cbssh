package model

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	TunnelTypeLocal   = "local"
	TunnelTypeRemote  = "remote"
	TunnelTypeDynamic = "dynamic"
)

// Config is the complete cbssh tunnel manifest. SSH connection settings are
// deliberately absent and remain owned by OpenSSH.
type Config struct {
	Version int      `toml:"version"`
	Tunnels []Tunnel `toml:"tunnels"`
}

// Tunnel describes one named OpenSSH forwarding rule.
type Tunnel struct {
	Name       string `toml:"name" json:"name"`
	Host       string `toml:"host" json:"host"`
	Type       string `toml:"type" json:"type"`
	BindHost   string `toml:"bind_host" json:"bind_host"`
	BindPort   int    `toml:"bind_port" json:"bind_port"`
	TargetHost string `toml:"target_host,omitempty" json:"target_host,omitempty"`
	TargetPort int    `toml:"target_port,omitempty" json:"target_port,omitempty"`
}

func (t *Tunnel) Normalize() {
	t.Name = strings.TrimSpace(t.Name)
	t.Host = strings.TrimSpace(t.Host)
	t.Type = strings.TrimSpace(t.Type)
	t.BindHost = strings.TrimSpace(t.BindHost)
	t.TargetHost = strings.TrimSpace(t.TargetHost)
	if t.BindHost == "" {
		t.BindHost = "127.0.0.1"
	}
}

func (t Tunnel) ForwardFlag() string {
	switch t.Type {
	case TunnelTypeLocal:
		return "-L"
	case TunnelTypeRemote:
		return "-R"
	case TunnelTypeDynamic:
		return "-D"
	default:
		return ""
	}
}

func (t Tunnel) ForwardSpec() (string, error) {
	bind := forwardAddress(t.BindHost, t.BindPort)
	switch t.Type {
	case TunnelTypeLocal, TunnelTypeRemote:
		return bind + ":" + forwardAddress(t.TargetHost, t.TargetPort), nil
	case TunnelTypeDynamic:
		return bind, nil
	default:
		return "", fmt.Errorf("unsupported tunnel type %q", t.Type)
	}
}

func forwardAddress(host string, port int) string {
	if strings.Contains(host, ":") && !(strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]")) {
		host = "[" + host + "]"
	}
	return host + ":" + strconv.Itoa(port)
}
