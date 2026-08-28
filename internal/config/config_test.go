package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CmdBlockZQG/cbssh/internal/model"
)

func TestLoadNormalizesBindHost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tunnels.toml")
	data := "version = 1\n[[tunnels]]\nname = \"db\"\nhost = \"prod\"\ntype = \"local\"\nbind_port = 15432\ntarget_host = \"db\"\ntarget_port = 5432\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Tunnels[0].BindHost != "127.0.0.1" {
		t.Fatalf("BindHost = %q", cfg.Tunnels[0].BindHost)
	}
}

func TestValidateRejectsDynamicTarget(t *testing.T) {
	tun := model.Tunnel{Name: "socks", Host: "prod", Type: model.TunnelTypeDynamic, BindHost: "127.0.0.1", BindPort: 1080, TargetHost: "unexpected"}
	err := Validate(model.Config{Version: 1, Tunnels: []model.Tunnel{tun}})
	if err == nil || !strings.Contains(err.Error(), "must not define a target") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tunnels.toml")
	if err := os.WriteFile(path, []byte("version = 1\nunexpected = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "unknown config fields") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestSelectRequiresExplicitAll(t *testing.T) {
	cfg := model.Config{Version: 1, Tunnels: []model.Tunnel{{Name: "one"}, {Name: "two"}}}
	if _, err := Select(cfg, nil, false); err == nil {
		t.Fatal("Select() error = nil without names or --all")
	}
	selected, err := Select(cfg, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 {
		t.Fatalf("selected = %d, want 2", len(selected))
	}
}

func TestInitNeverOverwritesExistingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tunnels.toml")
	if err := Init(path); err != nil {
		t.Fatal(err)
	}
	if err := Init(path); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("second Init() error = %v, want ErrAlreadyExists", err)
	}
}
