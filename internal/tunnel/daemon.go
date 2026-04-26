package tunnel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cmdblock/cbssh/internal/config"
	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/platform"
	"github.com/cmdblock/cbssh/internal/sshclient"
	"github.com/cmdblock/cbssh/internal/state"
)

// RunDaemon owns the SSH chain, live listeners, and control socket for a run.
func RunDaemon(ctx context.Context, opts DaemonOptions) error {
	if err := opts.normalize(); err != nil {
		return err
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
	logPath := filepath.Join(opts.LogDir, opts.RunID+".log")
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

func daemonLogDir() string {
	if path := os.Getenv("CBSSH_LOG_DIR"); path != "" {
		return path
	}
	return platform.DefaultLogDir()
}
