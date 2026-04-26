package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/platform"
)

func TestCleanupStaleKeepsMatchingProcessKey(t *testing.T) {
	processKey, err := platform.ProcessKey(os.Getpid())
	if err != nil {
		t.Skipf("process key is unavailable: %v", err)
	}
	path := filepath.Join(t.TempDir(), "state.json")
	entry := model.TunnelRuntime{
		ID:         "host/tunnel/run",
		RunID:      "run",
		HostName:   "host",
		TunnelName: "tunnel",
		PID:        os.Getpid(),
		ProcessKey: processKey,
		StartedAt:  time.Now(),
	}
	if err := Save(path, model.State{Version: 1, Tunnels: []model.TunnelRuntime{entry}}); err != nil {
		t.Fatal(err)
	}

	st, stale, err := CleanupStale(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 0 {
		t.Fatalf("stale = %#v, want none", stale)
	}
	if len(st.Tunnels) != 1 {
		t.Fatalf("active tunnels = %d, want 1", len(st.Tunnels))
	}
}

func TestCleanupStaleRemovesMismatchedProcessKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	entry := model.TunnelRuntime{
		ID:         "host/tunnel/run",
		RunID:      "run",
		HostName:   "host",
		TunnelName: "tunnel",
		PID:        os.Getpid(),
		ProcessKey: "definitely-not-this-process",
		StartedAt:  time.Now(),
	}
	if err := Save(path, model.State{Version: 1, Tunnels: []model.TunnelRuntime{entry}}); err != nil {
		t.Fatal(err)
	}

	st, stale, err := CleanupStale(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 1 {
		t.Fatalf("stale = %d, want 1", len(stale))
	}
	if len(st.Tunnels) != 0 {
		t.Fatalf("active tunnels = %d, want 0", len(st.Tunnels))
	}
}

func TestCleanupStaleRemovesEntriesWithoutProcessKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	entry := model.TunnelRuntime{
		ID:         "host/tunnel/run",
		RunID:      "run",
		HostName:   "host",
		TunnelName: "tunnel",
		PID:        os.Getpid(),
		StartedAt:  time.Now(),
	}
	if err := Save(path, model.State{Version: 1, Tunnels: []model.TunnelRuntime{entry}}); err != nil {
		t.Fatal(err)
	}

	st, stale, err := CleanupStale(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 1 {
		t.Fatalf("stale = %d, want 1", len(stale))
	}
	if len(st.Tunnels) != 0 {
		t.Fatalf("active tunnels = %d, want 0", len(st.Tunnels))
	}
}
