package tunnel

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/cmdblock/cbssh/internal/atomicfile"
	"github.com/cmdblock/cbssh/internal/platform"
)

type startupStatus struct {
	Error string `json:"error,omitempty"`
}

// startupStatusPath is scoped by run ID so cold starts never affect reused daemons.
func startupStatusPath(statePath string, runID string) string {
	statePath = platform.ExpandPath(statePath)
	if statePath == "" {
		statePath = platform.DefaultStatePath()
	}
	name := hashedRunFileName(runID, ".json")
	return filepath.Join(filepath.Dir(statePath), "startup", name)
}

func writeStartupError(statePath string, runID string, startupErr error) error {
	if startupErr == nil {
		return nil
	}
	path := startupStatusPath(statePath, runID)
	data, err := json.Marshal(startupStatus{Error: startupErr.Error()})
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicfile.WriteFile(path, ".startup-*.json", data, 0o600)
}

func readStartupError(statePath string, runID string) (error, bool, error) {
	data, err := os.ReadFile(startupStatusPath(statePath, runID))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var status startupStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, false, err
	}
	if status.Error == "" {
		return nil, true, nil
	}
	return errors.New(status.Error), true, nil
}

func removeStartupStatus(statePath string, runID string) error {
	err := os.Remove(startupStatusPath(statePath, runID))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
