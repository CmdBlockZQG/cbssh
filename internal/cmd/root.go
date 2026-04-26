package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/cmdblock/cbssh/internal/config"
	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/platform"
	"github.com/cmdblock/cbssh/internal/sshclient"
	"github.com/cmdblock/cbssh/internal/state"
	"github.com/cmdblock/cbssh/internal/tui"
	"github.com/cmdblock/cbssh/internal/tunnel"
)

const cliBold = "\033[1m"
const cliReset = "\033[0m"

type app struct {
	version    string
	configPath string
	statePath  string
}

func NewRootCommand(version string) *cobra.Command {
	a := &app{version: version}
	root := &cobra.Command{
		Use:           "cbssh",
		Short:         "Manage SSH hosts and tunnels",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
		RunE: func(cmd *cobra.Command, args []string) error {
			return tui.Run(cmd.Context(), a.configPath, a.statePath)
		},
	}
	root.PersistentFlags().StringVar(&a.configPath, "config", platform.DefaultConfigPath(), "Path to the cbssh config file")
	root.PersistentFlags().StringVar(&a.statePath, "state", platform.DefaultStatePath(), "Path to the cbssh state file")

	root.AddCommand(a.newTUICommand())
	root.AddCommand(a.newLSCommand())
	root.AddCommand(a.newInfoCommand())
	root.AddCommand(a.newConnectCommand())
	root.AddCommand(a.newTunnelCommand())
	root.AddCommand(a.newStatusCommand())
	root.AddCommand(a.newStopCommand())
	root.AddCommand(a.newConfigCommand())
	root.AddCommand(a.newDaemonCommand())
	return root
}

func (a *app) newTUICommand() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Open the interactive TUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			return tui.Run(cmd.Context(), a.configPath, a.statePath)
		},
	}
}

func (a *app) newLSCommand() *cobra.Command {
	var sortMode string
	c := &cobra.Command{
		Use:   "ls",
		Short: "List SSH hosts",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(a.configPath)
			if err != nil {
				return err
			}
			st, err := state.Load(a.statePath)
			if err != nil {
				return err
			}
			hosts := append([]model.Host(nil), cfg.Hosts...)
			switch sortMode {
			case "", "recent":
				sort.SliceStable(hosts, func(i, j int) bool {
					return st.Hosts[hosts[i].Name].LastUsed.After(st.Hosts[hosts[j].Name].LastUsed)
				})
			case "name":
				sort.SliceStable(hosts, func(i, j int) bool { return hosts[i].Name < hosts[j].Name })
			default:
				return fmt.Errorf("unsupported sort mode %q", sortMode)
			}
			if len(hosts) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No hosts configured.")
				return nil
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s%-16s %-21s %-10s %-16s %-7s %s%s\n",
				cliBold, "NAME", "HOST", "USER", "JUMP", "TUNNELS", "LAST USED", cliReset)
			for _, host := range hosts {
				fmt.Fprintf(out, "%-16s %-21s %-10s %-16s %-7d %s\n",
					host.Name,
					host.Address(),
					host.User,
					emptyDash(host.Jump),
					len(host.Tunnels),
					formatTime(st.Hosts[host.Name].LastUsed),
				)
			}
			return nil
		},
	}
	c.Flags().StringVar(&sortMode, "sort", "recent", "Sort hosts by recent or name")
	return c
}

func (a *app) newInfoCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Short: "Show SSH host details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(a.configPath)
			if err != nil {
				return err
			}
			host, ok := config.ResolveHost(cfg, args[0])
			if !ok {
				return fmt.Errorf("host %q not found", args[0])
			}
			chain, err := config.ResolveJumpNames(cfg, host.Name)
			if err != nil {
				return err
			}
			st, _, err := tunnel.Status(a.statePath, host.Name)
			if err != nil {
				return err
			}
			active := map[string]model.TunnelRuntime{}
			for _, entry := range st.Tunnels {
				active[entry.TunnelName] = entry
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%sName:%s %s\n", cliBold, cliReset, host.Name)
			fmt.Fprintf(out, "%sHost:%s %s\n", cliBold, cliReset, host.Address())
			fmt.Fprintf(out, "%sUser:%s %s\n", cliBold, cliReset, host.User)
			fmt.Fprintf(out, "%sJump:%s %s\n", cliBold, cliReset, strings.Join(chain, " -> "))
			fmt.Fprintf(out, "%sAuth:%s %s\n", cliBold, cliReset, authSummary(host))
			if len(host.Tunnels) == 0 {
				fmt.Fprintf(out, "%sTunnels:%s none\n", cliBold, cliReset)
				return nil
			}
			fmt.Fprintf(out, "%sTunnels:%s\n", cliBold, cliReset)
			fmt.Fprintf(out, "%s%-4s %-16s %-7s %-21s %-21s %-3s %-7s%s\n",
				cliBold, "NO", "NAME", "TYPE", "LISTEN", "TARGET", "DEF", "PID", cliReset)
			for i, tun := range host.Tunnels {
				_, isActive := active[tun.Name]
				pid := "-"
				if isActive {
					pid = fmt.Sprint(active[tun.Name].PID)
				}
				def := 0
				if tun.Default {
					def = 1
				}
				fmt.Fprintf(out, " %-3d %-16s %-7s %-21s %-21s %-3d %-7s\n",
					i+1,
					tun.Name,
					tun.Type,
					tun.ListenAddress(),
					emptyDash(tun.TargetAddress()),
					def,
					pid,
				)
			}
			return nil
		},
	}
}

func (a *app) newConnectCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "connect <name>",
		Aliases: []string{"c"},
		Short:   "Open an interactive SSH session",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(a.configPath)
			if err != nil {
				return err
			}
			chain, err := config.ResolveChain(cfg, args[0])
			if err != nil {
				return err
			}
			_ = state.MarkHostUsed(a.statePath, args[0], time.Now())
			return sshclient.RunInteractive(cmd.Context(), cfg, chain)
		},
	}
}

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

func (a *app) newConfigCommand() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage cbssh configuration",
	}
	configCmd.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Print the config file path",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), platform.ExpandPath(a.configPath))
			return nil
		},
	})
	configCmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Create an empty config file if it does not exist",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.Ensure(a.configPath); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Config ready at %s\n", platform.ExpandPath(a.configPath))
			return nil
		},
	})
	configCmd.AddCommand(&cobra.Command{
		Use:   "validate",
		Short: "Validate the config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := config.Load(a.configPath); err != nil {
				return err
			}
			for _, warning := range config.ValidateFilePermissions(a.configPath) {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %s\n", warning)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Config is valid.")
			return nil
		},
	})
	configCmd.AddCommand(&cobra.Command{
		Use:   "edit",
		Short: "Open the config file in $EDITOR",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.Ensure(a.configPath); err != nil {
				return err
			}
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vi"
			}
			editorCmd := exec.CommandContext(cmd.Context(), editor, platform.ExpandPath(a.configPath))
			editorCmd.Stdin = os.Stdin
			editorCmd.Stdout = os.Stdout
			editorCmd.Stderr = os.Stderr
			return editorCmd.Run()
		},
	})
	return configCmd
}

func (a *app) newDaemonCommand() *cobra.Command {
	daemonCmd := &cobra.Command{
		Use:    "daemon",
		Short:  "Run internal background daemons",
		Hidden: true,
	}
	var hostName string
	var runID string
	var tunnelsRaw string
	tunnelCmd := &cobra.Command{
		Use:   "tunnel",
		Short: "Run a background tunnel daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGTERM, syscall.SIGINT)
			defer stop()
			return tunnel.RunDaemon(ctx, tunnel.DaemonOptions{
				ConfigPath:  a.configPath,
				StatePath:   a.statePath,
				HostName:    hostName,
				TunnelNames: tunnel.SplitTunnelNames(tunnelsRaw),
				RunID:       runID,
			})
		},
	}
	tunnelCmd.Flags().StringVar(&hostName, "host", "", "Host name")
	tunnelCmd.Flags().StringVar(&runID, "run-id", "", "Daemon run id")
	tunnelCmd.Flags().StringVar(&tunnelsRaw, "tunnels", "", "Comma-separated tunnel names")
	_ = tunnelCmd.MarkFlagRequired("host")
	_ = tunnelCmd.MarkFlagRequired("run-id")
	daemonCmd.AddCommand(tunnelCmd)
	return daemonCmd
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

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Format("2006-01-02 15:04:05")
}

func authSummary(host model.Host) string {
	switch host.Auth.Type {
	case model.AuthTypePassword:
		return "password ******"
	case model.AuthTypeKey:
		return fmt.Sprintf("key %s", host.Auth.KeyPath)
	default:
		return host.Auth.Type
	}
}
