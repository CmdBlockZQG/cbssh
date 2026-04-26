package tunnel

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/state"
)

func startManagedTunnels(ctx context.Context, client dialer, opts DaemonOptions, tunnels []model.Tunnel, jumpNames []string, processKey string, controlPath string, logPath string, active map[string]managedTunnel) ([]model.TunnelRuntime, error) {
	seen := map[string]bool{}
	for _, tun := range tunnels {
		if active[tun.Name].Closer != nil {
			return nil, fmt.Errorf("tunnel %s is already active on %s", tun.Name, opts.HostName)
		}
		if seen[tun.Name] {
			return nil, fmt.Errorf("duplicate tunnel %q", tun.Name)
		}
		seen[tun.Name] = true
	}

	started := make([]managedTunnel, 0, len(tunnels))
	for _, tun := range tunnels {
		closer, err := startRuntimeTunnel(ctx, client, tun)
		if err != nil {
			rollbackManagedTunnels(active, started)
			return nil, err
		}
		entry := model.TunnelRuntime{
			ID:          fmt.Sprintf("%s/%s/%s", opts.HostName, tun.Name, opts.RunID),
			RunID:       opts.RunID,
			HostName:    opts.HostName,
			TunnelName:  tun.Name,
			Type:        tun.Type,
			PID:         os.Getpid(),
			ProcessKey:  processKey,
			ControlPath: controlPath,
			ListenHost:  tun.ListenHost,
			ListenPort:  tun.ListenPort,
			TargetHost:  tun.TargetHost,
			TargetPort:  tun.TargetPort,
			JumpChain:   jumpNames,
			StartedAt:   time.Now(),
			LogPath:     logPath,
		}
		managed := managedTunnel{Closer: closer, Entry: entry}
		active[tun.Name] = managed
		started = append(started, managed)
	}

	entries := make([]model.TunnelRuntime, 0, len(started))
	for _, managed := range started {
		entries = append(entries, managed.Entry)
	}

	// State is written only after every listener is live. If persistence fails,
	// rollback closes the listeners so callers never see untracked tunnels.
	if err := state.AddTunnels(opts.StatePath, entries); err != nil {
		rollbackManagedTunnels(active, started)
		return nil, err
	}
	return entries, nil
}

func stopManagedTunnels(statePath string, active map[string]managedTunnel, names []string) ([]model.TunnelRuntime, error) {
	if len(names) == 0 {
		names = make([]string, 0, len(active))
		for name := range active {
			names = append(names, name)
		}
	}
	var stopped []model.TunnelRuntime
	for _, name := range names {
		managed, ok := active[name]
		if !ok {
			continue
		}
		_ = managed.Closer.Close()
		delete(active, name)
		stopped = append(stopped, managed.Entry)
	}
	if len(stopped) > 0 {
		if err := state.RemoveEntries(statePath, stopped); err != nil {
			return stopped, err
		}
	}
	return stopped, nil
}

func rollbackManagedTunnels(active map[string]managedTunnel, tunnels []managedTunnel) {
	for _, managed := range tunnels {
		_ = managed.Closer.Close()
		delete(active, managed.Entry.TunnelName)
	}
}

func closeManagedTunnels(active map[string]managedTunnel) {
	for name, managed := range active {
		_ = managed.Closer.Close()
		delete(active, name)
	}
}
