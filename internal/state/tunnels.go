package state

import "github.com/cmdblock/cbssh/internal/model"

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
