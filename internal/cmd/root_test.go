package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListAndStoppedStatus(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "tunnels.toml")
	data := "version = 1\n[[tunnels]]\nname = \"db\"\nhost = \"prod\"\ntype = \"local\"\nbind_port = 15432\ntarget_host = \"db\"\ntarget_port = 5432\n"
	if err := os.WriteFile(configPath, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, commandName := range []string{"list", "ls"} {
		output := execute(t, "--dir", dir, commandName)
		if !strings.Contains(output, "db") || !strings.Contains(output, "-L 127.0.0.1:15432:db:5432") {
			t.Fatalf("%s output = %q", commandName, output)
		}
	}
	output := execute(t, "--dir", dir, "status")
	if !strings.Contains(output, "stopped") {
		t.Fatalf("status output = %q", output)
	}
}

func TestConfigInitIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tunnels.toml")
	execute(t, "--dir", dir, "init")
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	output := execute(t, "--dir", dir, "init")
	if !strings.Contains(output, "already exists") {
		t.Fatalf("second init output = %q", output)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("second init changed the existing config")
	}
}

func execute(t *testing.T, args ...string) string {
	t.Helper()
	command := NewRootCommand("test")
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs(args)
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute(%v): %v", args, err)
	}
	return output.String()
}
