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

	st, _, err := state.CleanupStale(opts.StatePath)
	if err != nil {
		return nil, err
	}
	for _, tun := range selected {
		for _, entry := range state.FindActive(st, opts.HostName, []string{tun.Name}) {
			return nil, fmt.Errorf("tunnel %s is already active on %s", entry.TunnelName, entry.HostName)
		}
	}

	var started []model.TunnelRuntime
	for _, tun := range selected {
		entry, err := startOne(ctx, opts, tun.Name)
		if err != nil {
			if len(started) > 0 {
				names := make([]string, 0, len(started))
				for _, startedEntry := range started {
					names = append(names, startedEntry.TunnelName)
				}
				_, _ = Stop(context.Background(), opts.StatePath, opts.HostName, names)
			}
			return started, err
		}
		started = append(started, entry)
	}
	return started, nil
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

	var closers []io.Closer
	var entries []model.TunnelRuntime
	for _, tun := range selected {
		closer, err := startRuntimeTunnel(ctx, client.Target(), tun)
		if err != nil {
			closeAll(closers)
			return err
		}
		closers = append(closers, closer)
		entries = append(entries, model.TunnelRuntime{
			ID:         fmt.Sprintf("%s/%s/%s", opts.HostName, tun.Name, opts.RunID),
			RunID:      opts.RunID,
			HostName:   opts.HostName,
			TunnelName: tun.Name,
			Type:       tun.Type,
			PID:        os.Getpid(),
			ListenHost: tun.ListenHost,
			ListenPort: tun.ListenPort,
			TargetHost: tun.TargetHost,
			TargetPort: tun.TargetPort,
			JumpChain:  jumpNames,
			StartedAt:  time.Now(),
			LogPath:    filepath.Join(platform.DefaultLogDir(), opts.RunID+".log"),
		})
	}
	if err := state.AddTunnels(opts.StatePath, entries); err != nil {
		closeAll(closers)
		return err
	}
	defer state.RemoveByRunID(opts.StatePath, opts.RunID)
	defer closeAll(closers)

	sshDone := make(chan error, 1)
	go func() {
		sshDone <- client.Target().Wait()
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-sshDone:
		if err != nil {
			return fmt.Errorf("SSH connection closed: %w", err)
		}
		return errors.New("SSH connection closed")
	}
}

func Stop(ctx context.Context, statePath string, hostName string, tunnelNames []string) ([]model.TunnelRuntime, error) {
	if statePath == "" {
		statePath = platform.DefaultStatePath()
	}
	st, err := state.Load(statePath)
	if err != nil {
		return nil, err
	}
	targets := state.FindActive(st, hostName, tunnelNames)
	if len(targets) == 0 {
		return nil, nil
	}

	pids := map[int]bool{}
	for _, entry := range targets {
		pids[entry.PID] = true
	}
	for pid := range pids {
		_ = platform.KillProcess(pid)
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

func startOne(ctx context.Context, opts StartOptions, tunnelName string) (model.TunnelRuntime, error) {
	exe, err := os.Executable()
	if err != nil {
		return model.TunnelRuntime{}, err
	}
	runID := fmt.Sprintf("%d-%d-%s-%s", time.Now().UnixNano(), os.Getpid(), opts.HostName, tunnelName)
	if err := os.MkdirAll(opts.LogDir, 0o700); err != nil {
		return model.TunnelRuntime{}, err
	}
	logPath := filepath.Join(opts.LogDir, runID+".log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return model.TunnelRuntime{}, err
	}
	defer logFile.Close()

	args := []string{
		"daemon", "tunnel",
		"--config", opts.ConfigPath,
		"--state", opts.StatePath,
		"--host", opts.HostName,
		"--run-id", runID,
		"--tunnels", tunnelName,
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return model.TunnelRuntime{}, err
	}
	defer devNull.Close()
	cmd.Stdin = devNull
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "CBSSH_LOG_DIR="+opts.LogDir)
	platform.DetachCommand(cmd)
	if err := cmd.Start(); err != nil {
		return model.TunnelRuntime{}, err
	}
	pid := cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		return model.TunnelRuntime{}, err
	}

	deadline := time.NewTimer(opts.Timeout)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		st, _, err := state.CleanupStale(opts.StatePath)
		if err != nil {
			return model.TunnelRuntime{}, err
		}
		for _, entry := range st.Tunnels {
			if entry.RunID == runID {
				entry.LogPath = logPath
				return entry, nil
			}
		}
		if !platform.ProcessExists(pid) {
			return model.TunnelRuntime{}, fmt.Errorf("tunnel process exited before becoming ready; see %s", logPath)
		}
		select {
		case <-ctx.Done():
			return model.TunnelRuntime{}, ctx.Err()
		case <-deadline.C:
			_ = platform.TerminateProcess(pid)
			return model.TunnelRuntime{}, fmt.Errorf("timed out waiting for tunnel readiness; see %s", logPath)
		case <-tick.C:
		}
	}
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
	go acceptLoop(ctx, listener, func(local net.Conn) {
		remote, err := client.Dial("tcp", tun.TargetAddress())
		if err != nil {
			_ = local.Close()
			return
		}
		pipe(local, remote)
	})
	return listener, nil
}

func startRemote(ctx context.Context, client dialer, tun model.Tunnel) (io.Closer, error) {
	listener, err := client.Listen("tcp", tun.ListenAddress())
	if err != nil {
		return nil, err
	}
	go acceptLoop(ctx, listener, func(remote net.Conn) {
		local, err := net.Dial("tcp", tun.TargetAddress())
		if err != nil {
			_ = remote.Close()
			return
		}
		pipe(local, remote)
	})
	return listener, nil
}

func startDynamic(ctx context.Context, client dialer, tun model.Tunnel) (io.Closer, error) {
	listener, err := net.Listen("tcp", tun.ListenAddress())
	if err != nil {
		return nil, err
	}
	go acceptLoop(ctx, listener, func(local net.Conn) {
		handleSOCKS5(local, client)
	})
	return listener, nil
}

func acceptLoop(ctx context.Context, listener net.Listener, handle func(net.Conn)) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				return
			}
		}
		go handle(conn)
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

func closeAll(closers []io.Closer) {
	for _, closer := range closers {
		_ = closer.Close()
	}
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
