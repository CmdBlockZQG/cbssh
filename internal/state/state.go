package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/platform"
)

func Empty() model.State {
	return model.State{
		Version: 1,
		Hosts:   map[string]model.HostRuntime{},
	}
}

func Load(path string) (model.State, error) {
	path = platform.ExpandPath(path)
	if path == "" {
		path = platform.DefaultStatePath()
	}
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
	if st.Version == 0 {
		st.Version = 1
	}
	if st.Hosts == nil {
		st.Hosts = map[string]model.HostRuntime{}
	}
	return st, nil
}

func Save(path string, st model.State) error {
	path = platform.ExpandPath(path)
	if path == "" {
		path = platform.DefaultStatePath()
	}
	if st.Version == 0 {
		st.Version = 1
	}
	if st.Hosts == nil {
		st.Hosts = map[string]model.HostRuntime{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".state-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func MarkHostUsed(path string, name string, usedAt time.Time) error {
	st, err := Load(path)
	if err != nil {
		return err
	}
	if st.Hosts == nil {
		st.Hosts = map[string]model.HostRuntime{}
	}
	st.Hosts[name] = model.HostRuntime{LastUsed: usedAt}
	return Save(path, st)
}

func UpsertTunnel(path string, entry model.TunnelRuntime) error {
	st, err := Load(path)
	if err != nil {
		return err
	}
	out := st.Tunnels[:0]
	for _, existing := range st.Tunnels {
		if existing.Key() == entry.Key() || existing.ID == entry.ID {
			continue
		}
		out = append(out, existing)
	}
	st.Tunnels = append(out, entry)
	return Save(path, st)
}

func AddTunnels(path string, entries []model.TunnelRuntime) error {
	st, err := Load(path)
	if err != nil {
		return err
	}
	remove := map[string]bool{}
	for _, entry := range entries {
		remove[entry.Key()] = true
		remove[entry.ID] = true
	}
	out := st.Tunnels[:0]
	for _, existing := range st.Tunnels {
		if remove[existing.Key()] || remove[existing.ID] {
			continue
		}
		out = append(out, existing)
	}
	st.Tunnels = append(out, entries...)
	return Save(path, st)
}

func RemoveByRunID(path string, runID string) error {
	st, err := Load(path)
	if err != nil {
		return err
	}
	out := st.Tunnels[:0]
	for _, entry := range st.Tunnels {
		if entry.RunID == runID {
			continue
		}
		out = append(out, entry)
	}
	st.Tunnels = out
	return Save(path, st)
}

func RemoveEntries(path string, entries []model.TunnelRuntime) error {
	remove := map[string]bool{}
	for _, entry := range entries {
		remove[entry.ID] = true
	}
	st, err := Load(path)
	if err != nil {
		return err
	}
	out := st.Tunnels[:0]
	for _, entry := range st.Tunnels {
		if remove[entry.ID] {
			continue
		}
		out = append(out, entry)
	}
	st.Tunnels = out
	return Save(path, st)
}

func CleanupStale(path string) (model.State, []model.TunnelRuntime, error) {
	st, err := Load(path)
	if err != nil {
		return model.State{}, nil, err
	}
	var stale []model.TunnelRuntime
	out := st.Tunnels[:0]
	for _, entry := range st.Tunnels {
		if platform.ProcessExists(entry.PID) {
			out = append(out, entry)
			continue
		}
		stale = append(stale, entry)
	}
	st.Tunnels = out
	if len(stale) > 0 {
		if err := Save(path, st); err != nil {
			return model.State{}, nil, err
		}
	}
	return st, stale, nil
}

func FindActive(st model.State, hostName string, tunnelNames []string) []model.TunnelRuntime {
	nameSet := map[string]bool{}
	for _, name := range tunnelNames {
		nameSet[name] = true
	}
	var out []model.TunnelRuntime
	for _, entry := range st.Tunnels {
		if hostName != "" && entry.HostName != hostName {
			continue
		}
		if len(nameSet) > 0 && !nameSet[entry.TunnelName] {
			continue
		}
		out = append(out, entry)
	}
	return out
}
