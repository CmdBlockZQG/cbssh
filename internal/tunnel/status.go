package tunnel

import (
	"context"
	"fmt"
	"sort"

	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/platform"
	"github.com/cmdblock/cbssh/internal/state"
)

// Stop stops matching tunnels through their daemon control sockets.
func Stop(ctx context.Context, statePath string, hostName string, tunnelNames []string) ([]model.TunnelRuntime, error) {
	if statePath == "" {
		statePath = platform.DefaultStatePath()
	}
	st, _, err := state.CleanupStale(statePath)
	if err != nil {
		return nil, err
	}
	targets := state.FindActive(st, hostName, tunnelNames)
	if len(targets) == 0 {
		return nil, nil
	}

	groups := map[string][]model.TunnelRuntime{}
	for _, entry := range targets {
		key := fmt.Sprintf("%s\x00%d\x00%s", entry.ControlPath, entry.PID, entry.ProcessKey)
		groups[key] = append(groups[key], entry)
	}
	for _, entries := range groups {
		first := entries[0]
		if first.ControlPath == "" {
			_ = platform.KillProcess(first.PID, first.ProcessKey)
			continue
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.TunnelName)
		}
		if _, err := sendControl(ctx, first, controlRequest{Op: "stop", Tunnels: names}); err != nil {
			return nil, err
		}
	}
	_ = state.RemoveEntries(statePath, targets)
	return targets, nil
}

// Status returns active tunnels after removing entries whose process is stale.
func Status(statePath string, hostName string) (model.State, []model.TunnelRuntime, error) {
	if statePath == "" {
		statePath = platform.DefaultStatePath()
	}
	st, stale, err := state.CleanupStale(statePath)
	if err != nil {
		return model.State{}, nil, err
	}
	if hostName == "" {
		sort.Slice(st.Tunnels, func(i, j int) bool {
			if st.Tunnels[i].HostName == st.Tunnels[j].HostName {
				return st.Tunnels[i].TunnelName < st.Tunnels[j].TunnelName
			}
			return st.Tunnels[i].HostName < st.Tunnels[j].HostName
		})
		return st, stale, nil
	}
	filtered := st.Tunnels[:0]
	for _, entry := range st.Tunnels {
		if entry.HostName == hostName {
			filtered = append(filtered, entry)
		}
	}
	st.Tunnels = filtered
	return st, stale, nil
}
