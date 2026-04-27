package tui

import (
	"bufio"
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cmdblock/cbssh/internal/model"
)

func TestParseCommandSplitsWhitespaceAndCommas(t *testing.T) {
	got := parseCommand("  s <name> <tun>,<tun2>  ")
	want := menuCommand{Action: "s", Args: []string{"<name>", "<tun>", "<tun2>"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseCommand() = %#v, want %#v", got, want)
	}
}

func TestResolveHostSelectorSupportsNumberNameCaseAndPrefix(t *testing.T) {
	cfg := testConfig()

	tests := []struct {
		name     string
		selector string
		want     int
	}{
		{name: "number", selector: "2", want: 1},
		{name: "exact", selector: "web1-db", want: 1},
		{name: "case insensitive", selector: "WEB1-DB", want: 1},
		{name: "unique prefix", selector: "ap", want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveHostSelector(cfg, tt.selector)
			if err != nil {
				t.Fatalf("resolveHostSelector() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolveHostSelector() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestResolveHostSelectorRejectsAmbiguousPrefix(t *testing.T) {
	_, err := resolveHostSelector(testConfig(), "web")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("resolveHostSelector() error = %v, want ambiguous error", err)
	}
}

func TestResolveTunnelSelectorsSupportsNumbersNamesAndDedupes(t *testing.T) {
	host := testConfig().Hosts[1]
	got, err := resolveTunnelSelectors(host, []string{"1", "tun2", "tun1"})
	if err != nil {
		t.Fatalf("resolveTunnelSelectors() error = %v", err)
	}
	want := []string{"tun1", "tun2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveTunnelSelectors() = %#v, want %#v", got, want)
	}
}

func TestResolveTunnelSelectorRejectsAmbiguousPrefix(t *testing.T) {
	host := testHost("web1-db")
	host.Tunnels = append(host.Tunnels, testTunnel("tun3"))
	_, err := resolveTunnelSelector(host, "t")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("resolveTunnelSelector() error = %v, want ambiguous error", err)
	}
}

func TestConnectHostReturnsSSHErrorWithoutExiting(t *testing.T) {
	oldRunInteractive := runInteractiveSSH
	oldExitProcess := exitProcess
	defer func() {
		runInteractiveSSH = oldRunInteractive
		exitProcess = oldExitProcess
	}()

	wantErr := errors.New("dial failed")
	exited := false
	runInteractiveSSH = func(context.Context, model.Config, []model.Host) error {
		return wantErr
	}
	exitProcess = func(int) {
		exited = true
	}

	cfg := testConfig()
	statePath := filepath.Join(t.TempDir(), "state.json")
	err := connectHost(context.Background(), bufio.NewReader(strings.NewReader("")), "", statePath, cfg, "web1")
	if err == nil || !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("connectHost() error = %v, want %v", err, wantErr)
	}
	if exited {
		t.Fatal("connectHost exited after SSH error")
	}
}

func TestConnectHostExitsAfterSuccessfulSession(t *testing.T) {
	oldRunInteractive := runInteractiveSSH
	oldExitProcess := exitProcess
	defer func() {
		runInteractiveSSH = oldRunInteractive
		exitProcess = oldExitProcess
	}()

	exitCode := -1
	runInteractiveSSH = func(context.Context, model.Config, []model.Host) error {
		return nil
	}
	exitProcess = func(code int) {
		exitCode = code
	}

	cfg := testConfig()
	statePath := filepath.Join(t.TempDir(), "state.json")
	if err := connectHost(context.Background(), bufio.NewReader(strings.NewReader("")), "", statePath, cfg, "web1"); err != nil {
		t.Fatalf("connectHost() error = %v, want nil", err)
	}
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
}

func testConfig() model.Config {
	return model.Config{
		Hosts: []model.Host{
			testHost("web1"),
			testHost("web1-db"),
			testHost("app"),
		},
	}
}

func testHost(name string) model.Host {
	return model.Host{
		Name: name,
		Host: "127.0.0.1",
		Port: 22,
		User: "user",
		Auth: model.Auth{
			Type:     model.AuthTypePassword,
			Password: "secret",
		},
		Tunnels: []model.Tunnel{
			testTunnel("tun1"),
			testTunnel("tun2"),
		},
	}
}

func testTunnel(name string) model.Tunnel {
	return model.Tunnel{
		Name:       name,
		Type:       model.TunnelTypeLocal,
		ListenHost: "127.0.0.1",
		ListenPort: 10000,
		TargetHost: "127.0.0.1",
		TargetPort: 10001,
		Default:    true,
	}
}
