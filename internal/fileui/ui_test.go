package fileui

import (
	"bufio"
	"context"
	"errors"
	"path/filepath"
	"strings"
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

func TestParseCommandPreservesXRemoteCommand(t *testing.T) {
	cmd := parseCommand("x find . -maxdepth 1")
	if cmd.action != "x" {
		t.Fatalf("action = %q, want x", cmd.action)
	}
	if len(cmd.args) != 1 || cmd.args[0] != "find . -maxdepth 1" {
		t.Fatalf("args = %#v, want [find . -maxdepth 1]", cmd.args)
	}
}

func TestParseCommandPreservesFilterText(t *testing.T) {
	cmd := parseCommand("/ app log")
	if cmd.action != "/" {
		t.Fatalf("action = %q, want /", cmd.action)
	}
	if len(cmd.args) != 1 || cmd.args[0] != "app log" {
		t.Fatalf("args = %#v, want [app log]", cmd.args)
	}

	cmd = parseCommand("/app")
	if cmd.action != "/" {
		t.Fatalf("action = %q, want /", cmd.action)
	}
	if len(cmd.args) != 1 || cmd.args[0] != "app" {
		t.Fatalf("args = %#v, want [app]", cmd.args)
	}
}

func TestRequireMaxArgsRejectsExtraArguments(t *testing.T) {
	err := requireMaxArgs(command{action: "u", args: []string{"a", "b", "c"}}, 2)
	if err == nil {
		t.Fatal("requireMaxArgs error = nil, want error")
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{value: "/home/app", want: "'/home/app'"},
		{value: "/home/app dir", want: "'/home/app dir'"},
		{value: "/tmp/a'b", want: "'/tmp/a'\\''b'"},
		{value: "", want: "''"},
	}
	for _, test := range tests {
		if got := shellQuote(test.value); got != test.want {
			t.Fatalf("shellQuote(%q) = %q, want %q", test.value, got, test.want)
		}
	}
}

func TestRemoteShellCommandChangesToQuotedCurrentDirectory(t *testing.T) {
	got := remoteShellCommand("/home/app dir", "ls -la")
	want := "cd '/home/app dir' && ls -la"
	if got != want {
		t.Fatalf("remoteShellCommand = %q, want %q", got, want)
	}
}

func TestRunRemoteCommandPromptsAndCancelsEmptyInput(t *testing.T) {
	u := &ui{reader: bufio.NewReader(strings.NewReader("\n"))}
	err := u.runRemoteCommand(context.Background(), "")
	if !errors.Is(err, errCanceled) {
		t.Fatalf("runRemoteCommand error = %v, want %v", err, errCanceled)
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

func TestApplyEntryFilterMatchesNamesCaseInsensitively(t *testing.T) {
	u := &ui{
		filter: "APP",
		entries: []filetransfer.Entry{
			{Name: "README.md", Path: "/home/app/README.md"},
			{Name: "app.log", Path: "/home/app/app.log"},
			{Name: "config.yaml", Path: "/home/app/config.yaml"},
			{Name: "Appfile", Path: "/home/app/Appfile"},
		},
	}
	u.applyEntryFilter()
	if len(u.visible) != 2 || u.visible[0].Name != "app.log" || u.visible[1].Name != "Appfile" {
		t.Fatalf("visible = %#v, want app.log and Appfile", u.visible)
	}
}

func TestResolveRemoteSelectorUsesFilteredVisibleEntries(t *testing.T) {
	u := &ui{
		cwd:    "/home/app",
		filter: "app",
		entries: []filetransfer.Entry{
			{Name: "README.md", Path: "/home/app/README.md"},
			{Name: "app.log", Path: "/home/app/app.log"},
			{Name: "docs", Path: "/home/app/docs"},
			{Name: "app.conf", Path: "/home/app/app.conf"},
		},
	}
	u.applyEntryFilter()
	got, err := u.resolveRemoteSelector("2")
	if err != nil {
		t.Fatalf("resolveRemoteSelector error = %v", err)
	}
	if got != "/home/app/app.conf" {
		t.Fatalf("resolveRemoteSelector = %q, want /home/app/app.conf", got)
	}
}

func TestSetFilterPromptsAndClearsEmptyInput(t *testing.T) {
	u := &ui{
		reader: bufio.NewReader(strings.NewReader("\n")),
		filter: "app",
		entries: []filetransfer.Entry{
			{Name: "README.md", Path: "/home/app/README.md"},
			{Name: "app.log", Path: "/home/app/app.log"},
		},
	}
	if err := u.setFilter(""); err != nil {
		t.Fatalf("setFilter error = %v", err)
	}
	if u.filter != "" {
		t.Fatalf("filter = %q, want empty", u.filter)
	}
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
