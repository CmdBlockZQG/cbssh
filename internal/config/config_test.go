package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmdblock/cbssh/internal/model"
)

func TestResolveChainOrdersJumpHostsBeforeTarget(t *testing.T) {
	cfg := model.Config{
		Hosts: []model.Host{
			host("target", "jump2"),
			host("jump1", ""),
			host("jump2", "jump1"),
		},
	}
	cfg.Normalize()

	chain, err := ResolveChain(cfg, "target")
	if err != nil {
		t.Fatalf("ResolveChain returned error: %v", err)
	}
	got := []string{chain[0].Name, chain[1].Name, chain[2].Name}
	want := []string{"jump1", "jump2", "target"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chain[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestValidateRejectsJumpCycle(t *testing.T) {
	cfg := model.Config{
		Hosts: []model.Host{
			host("a", "b"),
			host("b", "a"),
		},
	}
	cfg.Normalize()

	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "jump cycle") {
		t.Fatalf("Validate error = %v, want jump cycle error", err)
	}
}

func TestValidateRejectsUnsupportedHostKeyCheck(t *testing.T) {
	cfg := model.Config{
		HostKeyCheck: "strict",
		Hosts:        []model.Host{host("target", "")},
	}

	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "unsupported host_key_check") {
		t.Fatalf("Validate error = %v, want unsupported host_key_check error", err)
	}
}

func TestSelectTunnelsUsesDefaultWhenNamesAreEmpty(t *testing.T) {
	h := host("target", "")
	h.Tunnels = []model.Tunnel{
		tunnel("tun1", true),
		tunnel("tun2", false),
	}

	selected, err := SelectTunnels(h, nil)
	if err != nil {
		t.Fatalf("SelectTunnels returned error: %v", err)
	}
	if len(selected) != 1 || selected[0].Name != "tun1" {
		t.Fatalf("selected = %#v, want only tun1", selected)
	}
}

func TestSelectTunnelsRejectsDuplicateNames(t *testing.T) {
	h := host("target", "")
	h.Tunnels = []model.Tunnel{
		tunnel("tun1", true),
	}

	_, err := SelectTunnels(h, []string{"tun1", "tun1"})
	if err == nil || !strings.Contains(err.Error(), "duplicate selected tunnel") {
		t.Fatalf("SelectTunnels error = %v, want duplicate selected tunnel error", err)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	data := `
default_key_path = "~/.ssh/id_ed25519"
unexpected = true

[[hosts]]
name = "prod"
host = "10.0.0.1"
port = 22
user = "ubuntu"

[hosts.auth]
type = "password"
password = "secret"
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "unknown config fields") {
		t.Fatalf("Load error = %v, want unknown field error", err)
	}
}

func host(name string, jump string) model.Host {
	return model.Host{
		Name: name,
		Host: "127.0.0.1",
		Port: 22,
		User: "user",
		Jump: jump,
		Auth: model.Auth{
			Type:     model.AuthTypePassword,
			Password: "secret",
		},
	}
}

func tunnel(name string, def bool) model.Tunnel {
	return model.Tunnel{
		Name:       name,
		Type:       model.TunnelTypeLocal,
		ListenHost: "127.0.0.1",
		ListenPort: 10000,
		TargetHost: "127.0.0.1",
		TargetPort: 10001,
		Default:    def,
	}
}
