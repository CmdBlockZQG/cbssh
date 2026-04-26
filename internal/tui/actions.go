package tui

import (
	"bufio"
	"context"
	"fmt"
	"time"

	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/state"
	"github.com/cmdblock/cbssh/internal/tunnel"
)

func startTunnels(ctx context.Context, reader *bufio.Reader, statePath string, configPath string, cfg model.Config, args []string) error {
	host, names, err := selectHostAndTunnels(reader, cfg, args, "Tunnel names (blank for defaults)")
	if err != nil {
		return err
	}
	return startHostTunnels(ctx, statePath, configPath, host, names)
}

func startHostTunnels(ctx context.Context, statePath string, configPath string, host model.Host, selectors []string) error {
	names, err := resolveTunnelSelectors(host, selectors)
	if err != nil {
		return err
	}
	entries, err := tunnel.StartDetached(ctx, tunnel.StartOptions{
		ConfigPath:  configPath,
		StatePath:   statePath,
		HostName:    host.Name,
		TunnelNames: names,
	})
	if err != nil {
		return err
	}
	_ = state.MarkHostUsed(statePath, host.Name, time.Now())
	if len(entries) == 0 {
		fmt.Println("No inactive default tunnels to start.")
		return nil
	}
	for _, entry := range entries {
		fmt.Printf("Started %s/%s on %s (pid %d)\n", entry.HostName, entry.TunnelName, entry.ListenAddress(), entry.PID)
	}
	return nil
}

func stopTunnels(ctx context.Context, reader *bufio.Reader, statePath string, cfg model.Config, args []string) error {
	hostName := ""
	var names []string
	if len(args) > 0 {
		index, err := resolveHostSelector(cfg, args[0])
		if err != nil {
			return err
		}
		host := cfg.Hosts[index]
		hostName = host.Name
		if len(args) > 1 {
			names, err = resolveTunnelSelectors(host, args[1:])
			if err != nil {
				return err
			}
		} else {
			rawNames := splitArgs(promptString(reader, "Tunnel names (blank to stop all)", ""))
			names, err = resolveTunnelSelectors(host, rawNames)
			if err != nil {
				return err
			}
		}
	} else {
		selector := promptString(reader, "Host number/name (blank for all)", "")
		if selector != "" {
			index, err := resolveHostSelector(cfg, selector)
			if err != nil {
				return err
			}
			host := cfg.Hosts[index]
			hostName = host.Name
			rawNames := splitArgs(promptString(reader, "Tunnel names (blank to stop all)", ""))
			names, err = resolveTunnelSelectors(host, rawNames)
			if err != nil {
				return err
			}
		}
	}
	return stopSelectedTunnels(ctx, statePath, hostName, names)
}

func stopHostTunnels(ctx context.Context, statePath string, host model.Host, selectors []string) error {
	names, err := resolveTunnelSelectors(host, selectors)
	if err != nil {
		return err
	}
	return stopSelectedTunnels(ctx, statePath, host.Name, names)
}

func stopSelectedTunnels(ctx context.Context, statePath string, hostName string, names []string) error {
	entries, err := tunnel.Stop(ctx, statePath, hostName, names)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println("No active tunnels matched.")
		return nil
	}
	for _, entry := range entries {
		fmt.Printf("Stopped %s/%s (pid %d)\n", entry.HostName, entry.TunnelName, entry.PID)
	}
	return nil
}
