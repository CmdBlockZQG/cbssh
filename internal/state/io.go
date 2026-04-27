package state

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/cmdblock/cbssh/internal/atomicfile"
	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/platform"
)

func Load(path string) (model.State, error) {
	path = statePath(path)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return Empty(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return model.State{}, err
	}
	if len(data) == 0 {
		return Empty(), nil
	}
	var st model.State
	if err := json.Unmarshal(data, &st); err != nil {
		return model.State{}, err
	}
	return normalize(st), nil
}

func Save(path string, st model.State) error {
	path = statePath(path)
	st = normalize(st)
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicfile.WriteFile(path, ".state-*.json", data, 0o600)
}

func normalize(st model.State) model.State {
	if st.Version == 0 {
		st.Version = 1
	}
	if st.Hosts == nil {
		st.Hosts = map[string]model.HostRuntime{}
	}
	return st
}

func statePath(path string) string {
	path = platform.ExpandPath(path)
	if path == "" {
		return platform.DefaultStatePath()
	}
	return path
}
