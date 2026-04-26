package tunnel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cmdblock/cbssh/internal/config"
	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/platform"
	"github.com/cmdblock/cbssh/internal/sshclient"
	"github.com/cmdblock/cbssh/internal/state"
)

type StartOptions struct {
	ConfigPath  string
	StatePath   string
	LogDir      string
	HostName    string
	TunnelNames []string
	Timeout     time.Duration
}

type DaemonOptions struct {
	ConfigPath  string
	StatePath   string
	HostName    string
	TunnelNames []string
	RunID       string
}

type managedTunnel struct {
	Closer io.Closer
	Entry  model.TunnelRuntime
}

func StartDetached(ctx context.Context, opts StartOptions) ([]model.TunnelRuntime, error) {
	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Second
	}
	if opts.StatePath == "" {
		opts.StatePath = platform.DefaultStatePath()
	}
	if opts.ConfigPath == "" {
		opts.ConfigPath = platform.DefaultConfigPath()
	}
	if opts.LogDir == "" {
		opts.LogDir = platform.DefaultLogDir()
	}

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
	if reusable != nil {
		resp, err := sendControl(ctx, *reusable, controlRequest{Op: "start", Tunnels: pendingNames, TunnelDefs: pendingTunnels})
		if err == nil {
			return resp.Entries, nil
		}
		return nil, fmt.Errorf("reuse daemon %d: %w", reusable.PID, err)
	}
	return startDaemon(ctx, opts, pendingNames)
}

func RunDaemon(ctx context.Context, opts DaemonOptions) error {
	if opts.StatePath == "" {
		opts.StatePath = platform.DefaultStatePath()
	}
	if opts.ConfigPath == "" {
		opts.ConfigPath = platform.DefaultConfigPath()
	}
	if opts.RunID == "" {
		return errors.New("run id is required")
	}
	controlPath := controlSocketPath(opts.StatePath, opts.RunID)
	controlListener, err := listenControl(controlPath)
	if err != nil {
		return err
	}
	defer os.Remove(controlPath)
	defer controlListener.Close()
	daemonCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	commands := make(chan daemonCommand)
	go serveControl(daemonCtx, controlListener, commands)

	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return err
	}
	host, ok := config.ResolveHost(cfg, opts.HostName)
	if !ok {
		return fmt.Errorf("host %q not found", opts.HostName)
	}
	selected, err := config.SelectTunnels(host, opts.TunnelNames)
	if err != nil {
		return err
	}
	chain, err := config.ResolveChain(cfg, opts.HostName)
	if err != nil {
		return err
	}
	jumpNames, err := config.ResolveJumpNames(cfg, opts.HostName)
	if err != nil {
		return err
	}
	client, err := sshclient.DialChain(ctx, cfg, chain)
	if err != nil {
		return err
	}
	defer client.Close()

	processKey, err := platform.ProcessKey(os.Getpid())
	if err != nil {
		return fmt.Errorf("process identity unavailable: %w", err)
	}
	active := map[string]managedTunnel{}
	logPath := filepath.Join(platform.DefaultLogDir(), opts.RunID+".log")
	if _, err := startManagedTunnels(ctx, client.Target(), opts, selected, jumpNames, processKey, controlPath, logPath, active); err != nil {
		return err
	}
	defer state.RemoveByRunID(opts.StatePath, opts.RunID)
	defer closeManagedTunnels(active)

	sshDone := make(chan error, 1)
	go func() {
		sshDone <- client.Target().Wait()
	}()

	for {
		if len(active) == 0 {
			return nil
		}
		select {
		case command := <-commands:
			command.Response <- handleDaemonCommand(ctx, command.Request, opts, host, client.Target(), jumpNames, processKey, controlPath, logPath, active)
		case <-ctx.Done():
			return nil
		case err := <-sshDone:
			if err != nil {
				return fmt.Errorf("SSH connection closed: %w", err)
			}
			return errors.New("SSH connection closed")
		}
	}
}

func handleDaemonCommand(ctx context.Context, req controlRequest, opts DaemonOptions, host model.Host, client dialer, jumpNames []string, processKey string, controlPath string, logPath string, active map[string]managedTunnel) controlResponse {
	if req.ProcessKey != processKey {
		return controlResponse{Error: "process identity mismatch"}
	}
	switch req.Op {
	case "start":
		selected := req.TunnelDefs
		if len(selected) == 0 {
			var err error
			selected, err = config.SelectTunnels(host, req.Tunnels)
			if err != nil {
				return controlResponse{Error: err.Error()}
			}
		}
		entries, err := startManagedTunnels(ctx, client, opts, selected, jumpNames, processKey, controlPath, logPath, active)
		if err != nil {
			return controlResponse{Error: err.Error()}
		}
		return controlResponse{Entries: entries}
	case "stop":
		if _, err := stopManagedTunnels(opts.StatePath, active, req.Tunnels); err != nil {
			return controlResponse{Error: err.Error()}
		}
		return controlResponse{}
	default:
		return controlResponse{Error: fmt.Sprintf("unsupported control operation %q", req.Op)}
	}
}

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

func startDaemon(ctx context.Context, opts StartOptions, tunnelNames []string) ([]model.TunnelRuntime, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	runID := fmt.Sprintf("%d-%d-%s", time.Now().UnixNano(), os.Getpid(), opts.HostName)
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
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "CBSSH_LOG_DIR="+opts.LogDir)
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

	deadline := time.NewTimer(opts.Timeout)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
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
			return entries, nil
		}
		if !platform.ProcessMatches(pid, processKey) {
			return nil, fmt.Errorf("tunnel process exited before becoming ready; see %s", logPath)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			_ = platform.TerminateProcess(pid, processKey)
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

func startRuntimeTunnel(ctx context.Context, client dialer, tun model.Tunnel) (io.Closer, error) {
	switch tun.Type {
	case model.TunnelTypeLocal:
		return startLocal(ctx, client, tun)
	case model.TunnelTypeRemote:
		return startRemote(ctx, client, tun)
	case model.TunnelTypeDynamic:
		return startDynamic(ctx, client, tun)
	default:
		return nil, fmt.Errorf("unsupported tunnel type %q", tun.Type)
	}
}

type dialer interface {
	Dial(network, addr string) (net.Conn, error)
	Listen(network, addr string) (net.Listener, error)
}

func startLocal(ctx context.Context, client dialer, tun model.Tunnel) (io.Closer, error) {
	listener, err := net.Listen("tcp", tun.ListenAddress())
	if err != nil {
		return nil, err
	}
	runtime := newRuntimeTunnel(listener)
	go acceptLoop(ctx, runtime, func(local net.Conn) {
		remote, err := client.Dial("tcp", tun.TargetAddress())
		if err != nil {
			_ = local.Close()
			return
		}
		done := pipe(local, remote)
		<-done
	})
	return runtime, nil
}

func startRemote(ctx context.Context, client dialer, tun model.Tunnel) (io.Closer, error) {
	listener, err := client.Listen("tcp", tun.ListenAddress())
	if err != nil {
		return nil, err
	}
	runtime := newRuntimeTunnel(listener)
	go acceptLoop(ctx, runtime, func(remote net.Conn) {
		local, err := net.Dial("tcp", tun.TargetAddress())
		if err != nil {
			_ = remote.Close()
			return
		}
		done := pipe(local, remote)
		<-done
	})
	return runtime, nil
}

func startDynamic(ctx context.Context, client dialer, tun model.Tunnel) (io.Closer, error) {
	listener, err := net.Listen("tcp", tun.ListenAddress())
	if err != nil {
		return nil, err
	}
	runtime := newRuntimeTunnel(listener)
	go acceptLoop(ctx, runtime, func(local net.Conn) {
		handleSOCKS5(local, client)
	})
	return runtime, nil
}

type runtimeTunnel struct {
	listener net.Listener
	mu       sync.Mutex
	conns    map[net.Conn]bool
}

func newRuntimeTunnel(listener net.Listener) *runtimeTunnel {
	return &runtimeTunnel{
		listener: listener,
		conns:    map[net.Conn]bool{},
	}
}

func (t *runtimeTunnel) Close() error {
	err := t.listener.Close()
	t.mu.Lock()
	defer t.mu.Unlock()
	for conn := range t.conns {
		_ = conn.Close()
	}
	return err
}

func (t *runtimeTunnel) track(conn net.Conn) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.conns[conn] = true
}

func (t *runtimeTunnel) untrack(conn net.Conn) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.conns, conn)
}

func acceptLoop(ctx context.Context, tunnel *runtimeTunnel, handle func(net.Conn)) {
	for {
		conn, err := tunnel.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				return
			}
		}
		tunnel.track(conn)
		go func() {
			defer tunnel.untrack(conn)
			handle(conn)
		}()
	}
}

func pipe(a net.Conn, b net.Conn) <-chan struct{} {
	var once sync.Once
	done := make(chan struct{})
	closeBoth := func() {
		_ = a.Close()
		_ = b.Close()
		close(done)
	}
	go func() {
		_, _ = io.Copy(a, b)
		once.Do(closeBoth)
	}()
	go func() {
		_, _ = io.Copy(b, a)
		once.Do(closeBoth)
	}()
	return done
}

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
