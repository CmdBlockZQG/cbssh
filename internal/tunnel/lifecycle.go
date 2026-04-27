package tunnel

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cmdblock/cbssh/internal/config"
	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/platform"
	"github.com/cmdblock/cbssh/internal/state"
)

// StartDetached starts missing tunnels for a host and returns their runtime state.
func StartDetached(ctx context.Context, opts StartOptions) ([]model.TunnelRuntime, error) {
	opts.normalize()

	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return nil, err
	}
	host, ok := config.ResolveHost(cfg, opts.HostName)
	if !ok {
		return nil, fmt.Errorf("host %q not found", opts.HostName)
	}
	selected, err := config.SelectTunnels(host, opts.TunnelNames)
	if err != nil {
		return nil, err
	}
	defaultStart := len(opts.TunnelNames) == 0

	st, _, err := state.CleanupStale(opts.StatePath)
	if err != nil {
		return nil, err
	}
	activeByName := map[string]model.TunnelRuntime{}
	var reusable *model.TunnelRuntime
	for _, entry := range state.FindActive(st, opts.HostName, nil) {
		activeByName[entry.TunnelName] = entry
		if reusable == nil && entry.ControlPath != "" {
			candidate := entry
			reusable = &candidate
		}
	}

	// Default starts are idempotent: already-active defaults are skipped, while
	// explicitly requested active tunnels still return an error to expose typos.
	pendingNames := make([]string, 0, len(selected))
	pendingTunnels := make([]model.Tunnel, 0, len(selected))
	for _, tun := range selected {
		if entry, ok := activeByName[tun.Name]; ok {
			if defaultStart {
				continue
			}
			return nil, fmt.Errorf("tunnel %s is already active on %s", entry.TunnelName, entry.HostName)
		}
		pendingNames = append(pendingNames, tun.Name)
		pendingTunnels = append(pendingTunnels, tun)
	}
	if len(pendingNames) == 0 {
		return nil, nil
	}

	// Reusing an existing daemon keeps one SSH chain per host even as tunnels are
	// added incrementally from the CLI or TUI.
	if reusable != nil {
		resp, err := sendControl(ctx, *reusable, controlRequest{Op: "start", Tunnels: pendingNames, TunnelDefs: pendingTunnels})
		if err == nil {
			return resp.Entries, nil
		}
		return nil, fmt.Errorf("reuse daemon %d: %w", reusable.PID, err)
	}
	return startDaemon(ctx, opts, pendingNames)
}

func startDaemon(ctx context.Context, opts StartOptions, tunnelNames []string) ([]model.TunnelRuntime, error) {
	opts.normalize()

	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	runID := fmt.Sprintf("%d-%d-%s", time.Now().UnixNano(), os.Getpid(), opts.HostName)
	_ = removeStartupStatus(opts.StatePath, runID)
	if err := os.MkdirAll(opts.LogDir, 0o700); err != nil {
		return nil, err
	}
	logPath := filepath.Join(opts.LogDir, runID+".log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	defer logFile.Close()

	args := []string{
		"daemon", "tunnel",
		"--config", opts.ConfigPath,
		"--state", opts.StatePath,
		"--host", opts.HostName,
		"--run-id", runID,
		"--tunnels", strings.Join(tunnelNames, ","),
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return nil, err
	}
	defer devNull.Close()
	cmd.Stdin = devNull
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(), "CBSSH_LOG_DIR="+opts.LogDir)
	platform.DetachCommand(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	pid := cmd.Process.Pid
	processKey, err := platform.ProcessKey(pid)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("process identity unavailable for tunnel daemon: %w", err)
	}
	if err := cmd.Process.Release(); err != nil {
		_ = platform.KillProcess(pid, processKey)
		_ = cmd.Wait()
		return nil, err
	}

	daemonReady := false
	defer func() {
		if daemonReady {
			return
		}
		_ = platform.TerminateProcess(pid, processKey)
		_ = removeStartupStatus(opts.StatePath, runID)
	}()

	deadline := time.NewTimer(opts.Timeout)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		startupErr, found, err := readStartupError(opts.StatePath, runID)
		if err != nil {
			return nil, fmt.Errorf("read tunnel startup status: %w", err)
		}
		if found {
			_ = removeStartupStatus(opts.StatePath, runID)
			if startupErr != nil {
				return nil, fmt.Errorf("tunnel startup failed: %w; see %s", startupErr, logPath)
			}
			return nil, fmt.Errorf("tunnel startup failed; see %s", logPath)
		}

		st, _, err := state.CleanupStale(opts.StatePath)
		if err != nil {
			return nil, err
		}
		var entries []model.TunnelRuntime
		for _, entry := range st.Tunnels {
			if entry.RunID == runID {
				entries = append(entries, entry)
			}
		}
		if len(entries) == len(tunnelNames) {
			sort.Slice(entries, func(i, j int) bool {
				return tunnelOrder(tunnelNames, entries[i].TunnelName) < tunnelOrder(tunnelNames, entries[j].TunnelName)
			})
			daemonReady = true
			return entries, nil
		}
		if !platform.ProcessMatches(pid, processKey) {
			return nil, fmt.Errorf("tunnel process exited before becoming ready; see %s", logPath)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, fmt.Errorf("timed out waiting for tunnel readiness; see %s", logPath)
		case <-tick.C:
		}
	}
}

func tunnelOrder(names []string, name string) int {
	for i, candidate := range names {
		if candidate == name {
			return i
		}
	}
	return len(names)
}

// SplitTunnelNames parses comma-separated names used by the internal daemon flag.
func SplitTunnelNames(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	names := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			names = append(names, part)
		}
	}
	return names
}
