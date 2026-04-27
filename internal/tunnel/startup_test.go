package tunnel

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestStartupErrorRoundTrip(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	runID := "test-run"
	want := "listen tcp 127.0.0.1:1080: bind: address already in use"

	if err := writeStartupError(statePath, runID, errors.New(want)); err != nil {
		t.Fatal(err)
	}

	got, found, err := readStartupError(statePath, runID)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("readStartupError() found = false, want true")
	}
	if got == nil || got.Error() != want {
		t.Fatalf("readStartupError() error = %v, want %q", got, want)
	}

	if err := removeStartupStatus(statePath, runID); err != nil {
		t.Fatal(err)
	}
	_, found, err = readStartupError(statePath, runID)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("readStartupError() found = true after remove, want false")
	}
}
