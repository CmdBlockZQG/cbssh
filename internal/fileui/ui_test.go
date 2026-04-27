package fileui

import (
	"path/filepath"
	"testing"

	"github.com/cmdblock/cbssh/internal/filetransfer"
)

func TestParseCommandSplitsCommaAndWhitespace(t *testing.T) {
	cmd := parseCommand("d 1, logs/app.log")
	if cmd.action != "d" {
		t.Fatalf("action = %q, want d", cmd.action)
	}
	if len(cmd.args) != 2 || cmd.args[0] != "1" || cmd.args[1] != "logs/app.log" {
		t.Fatalf("args = %#v, want [1 logs/app.log]", cmd.args)
	}
}

func TestRequireMaxArgsRejectsExtraArguments(t *testing.T) {
	err := requireMaxArgs(command{action: "u", args: []string{"a", "b", "c"}}, 2)
	if err == nil {
		t.Fatal("requireMaxArgs error = nil, want error")
	}
}

func TestDownloadDestinationDefaultsInsideLocalDirectory(t *testing.T) {
	got := downloadDestination("/var/log/app.log", ".")
	want := filepath.Join(".", "app.log")
	if got != want {
		t.Fatalf("downloadDestination = %q, want %q", got, want)
	}
}

func TestDownloadDestinationUsesExplicitFilePath(t *testing.T) {
	got := downloadDestination("/var/log/app.log", "local.log")
	if got != "local.log" {
		t.Fatalf("downloadDestination = %q, want local.log", got)
	}
}

func TestApplyEntryFilterHidesDotFilesByDefault(t *testing.T) {
	u := &ui{
		entries: []filetransfer.Entry{
			{Name: ".env", Path: "/home/app/.env"},
			{Name: "app.log", Path: "/home/app/app.log"},
		},
	}
	u.applyEntryFilter()
	if len(u.visible) != 1 || u.visible[0].Name != "app.log" {
		t.Fatalf("visible = %#v, want only app.log", u.visible)
	}
	u.showDot = true
	u.applyEntryFilter()
	if len(u.visible) != 2 {
		t.Fatalf("visible entries = %d, want 2", len(u.visible))
	}
}

func TestResolveRemoteSelectorUsesVisibleEntries(t *testing.T) {
	u := &ui{
		cwd: "/home/app",
		entries: []filetransfer.Entry{
			{Name: ".env", Path: "/home/app/.env"},
			{Name: "app.log", Path: "/home/app/app.log"},
		},
	}
	u.applyEntryFilter()
	got, err := u.resolveRemoteSelector("1")
	if err != nil {
		t.Fatalf("resolveRemoteSelector error = %v", err)
	}
	if got != "/home/app/app.log" {
		t.Fatalf("resolveRemoteSelector = %q, want /home/app/app.log", got)
	}
}
