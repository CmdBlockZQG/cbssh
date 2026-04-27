package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cmdblock/cbssh/internal/state"
	"github.com/cmdblock/cbssh/internal/tunnel"
)

func (a *app) newTunnelCommand() *cobra.Command {
	tunnelCmd := &cobra.Command{
		Use:   "tunnel <name> [tunnel...]",
		Short: "Manage SSH tunnels",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return a.runTunnelStart(cmd, args[0], args[1:])
		},
	}
	tunnelCmd.AddCommand(&cobra.Command{
		Use:   "start <name> [tunnel...]",
		Short: "Start default or selected tunnels",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runTunnelStart(cmd, args[0], args[1:])
		},
	})
	tunnelCmd.AddCommand(&cobra.Command{
		Use:   "stop [name] [tunnel...]",
		Short: "Stop active tunnels",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runTunnelStop(cmd, args)
		},
	})
	tunnelCmd.AddCommand(&cobra.Command{
		Use:   "status [name]",
		Short: "Show active tunnels",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hostName := ""
			if len(args) == 1 {
				hostName = args[0]
			}
			return a.runTunnelStatus(cmd, hostName)
		},
	})
	tunnelCmd.AddCommand(&cobra.Command{
		Use:   "restart <name> [tunnel...]",
		Short: "Restart default or selected tunnels",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.runTunnelStop(cmd, args); err != nil {
				return err
			}
			return a.runTunnelStart(cmd, args[0], args[1:])
		},
	})
	return tunnelCmd
}

func (a *app) newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status [name]",
		Short: "Show active tunnels",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hostName := ""
			if len(args) == 1 {
				hostName = args[0]
			}
			return a.runTunnelStatus(cmd, hostName)
		},
	}
}

func (a *app) newStopCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stop [name] [tunnel...]",
		Short: "Stop active tunnels",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runTunnelStop(cmd, args)
		},
	}
}

func (a *app) runTunnelStart(cmd *cobra.Command, hostName string, names []string) error {
	entries, err := tunnel.StartDetached(cmd.Context(), tunnel.StartOptions{
		ConfigPath:  a.configPath,
		StatePath:   a.statePath,
		HostName:    hostName,
		TunnelNames: splitTunnelArgs(names),
	})
	if err != nil {
		return err
	}
	_ = state.MarkHostUsed(a.statePath, hostName, time.Now())
	if len(entries) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No inactive default tunnels to start.")
		return nil
	}
	for _, entry := range entries {
		fmt.Fprintf(cmd.OutOrStdout(), "Started %s/%s on %s (pid %d)\n", entry.HostName, entry.TunnelName, entry.ListenAddress(), entry.PID)
	}
	return nil
}

func (a *app) runTunnelStop(cmd *cobra.Command, args []string) error {
	hostName := ""
	var names []string
	if len(args) > 0 {
		hostName = args[0]
		names = splitTunnelArgs(args[1:])
	}
	stopped, err := tunnel.Stop(cmd.Context(), a.statePath, hostName, names)
	if err != nil {
		return err
	}
	if len(stopped) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No active tunnels matched.")
		return nil
	}
	for _, entry := range stopped {
		fmt.Fprintf(cmd.OutOrStdout(), "Stopped %s/%s (pid %d)\n", entry.HostName, entry.TunnelName, entry.PID)
	}
	return nil
}

func (a *app) runTunnelStatus(cmd *cobra.Command, hostName string) error {
	st, stale, err := tunnel.Status(a.statePath, hostName)
	if err != nil {
		return err
	}
	for _, entry := range stale {
		fmt.Fprintf(cmd.ErrOrStderr(), "Cleaned stale tunnel %s/%s (pid %d)\n", entry.HostName, entry.TunnelName, entry.PID)
	}
	if len(st.Tunnels) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No active tunnels.")
		return nil
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%s%-16s %-16s %-7s %-21s %-21s %-7s %-19s %s%s\n",
		cliBold, "HOST", "TUNNEL", "TYPE", "LISTEN", "TARGET", "PID", "STARTED", "LOG", cliReset)
	for _, entry := range st.Tunnels {
		fmt.Fprintf(out, "%-16s %-16s %-7s %-21s %-21s %-7d %-19s %s\n",
			entry.HostName,
			entry.TunnelName,
			entry.Type,
			entry.ListenAddress(),
			emptyDash(entry.TargetAddress()),
			entry.PID,
			formatTime(entry.StartedAt),
			entry.LogPath,
		)
	}
	return nil
}

func splitTunnelArgs(args []string) []string {
	return tunnel.SplitTunnelNames(strings.Join(args, ","))
}
