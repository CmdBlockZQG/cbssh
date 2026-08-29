package platform

import (
	"path/filepath"
	"testing"
)

func TestResolveLayoutUsesDefaultCbsshDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	wantDir := filepath.Join(home, ".cbssh")
	got := ResolveLayout("")
	if got.Dir != wantDir {
		t.Fatalf("ResolveLayout().Dir = %q, want %q", got.Dir, wantDir)
	}
	if want := filepath.Join(wantDir, "tunnels.toml"); got.ConfigPath != want {
		t.Fatalf("ResolveLayout().ConfigPath = %q, want %q", got.ConfigPath, want)
	}
	if want := filepath.Join(wantDir, "runtime.json"); got.StatePath != want {
		t.Fatalf("ResolveLayout().StatePath = %q, want %q", got.StatePath, want)
	}
}

func TestResolveLayoutExpandsExplicitDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	want := filepath.Join(home, ".custom-cbssh")
	if got := ResolveLayout("~/.custom-cbssh").Dir; got != want {
		t.Fatalf("ResolveLayout().Dir = %q, want %q", got, want)
	}
}
