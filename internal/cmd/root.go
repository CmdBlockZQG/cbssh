package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
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
	root.AddCommand(a.newShowCommand())
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
	var tag string
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
			if tag != "" {
				hosts = filterHostsByTag(hosts, tag)
			}
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
			w := newTable(cmd.OutOrStdout())
			fmt.Fprintln(w, "NAME\tHOST\tUSER\tJUMP\tTUNNELS\tTAGS\tLAST USED")
			for _, host := range hosts {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
					host.Name,
					host.Address(),
					host.User,
					emptyDash(host.Jump),
					len(host.Tunnels),
					emptyDash(strings.Join(host.Tags, ",")),
					formatTime(st.Hosts[host.Name].LastUsed),
				)
			}
			return w.Flush()
		},
	}
	c.Flags().StringVar(&sortMode, "sort", "recent", "Sort hosts by recent or name")
	c.Flags().StringVar(&tag, "tag", "", "Filter hosts by tag")
	return c
}

func (a *app) newShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
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
			fmt.Fprintf(out, "Name: %s\n", host.Name)
			fmt.Fprintf(out, "Host: %s\n", host.Address())
			fmt.Fprintf(out, "User: %s\n", host.User)
			fmt.Fprintf(out, "Jump chain: %s\n", strings.Join(chain, " -> "))
			fmt.Fprintf(out, "Tags: %s\n", emptyDash(strings.Join(host.Tags, ", ")))
			fmt.Fprintf(out, "Auth: %s\n", authSummary(host))
			if len(host.Tunnels) == 0 {
				fmt.Fprintln(out, "Tunnels: none")
				return nil
			}
			fmt.Fprintln(out, "Tunnels:")
			w := newTable(out)
			fmt.Fprintln(w, "NAME\tTYPE\tLISTEN\tTARGET\tDEFAULT\tACTIVE\tPID")
			for _, tun := range host.Tunnels {
				entry, isActive := active[tun.Name]
				pid := "-"
				if isActive {
					pid = fmt.Sprint(entry.PID)
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%t\t%t\t%s\n",
					tun.Name,
					tun.Type,
					tun.ListenAddress(),
					emptyDash(tun.TargetAddress()),
					tun.Default,
					isActive,
					pid,
				)
			}
			return w.Flush()
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
		TunnelNames: names,
	})
	if err != nil {
		return err
	}
	_ = state.MarkHostUsed(a.statePath, hostName, time.Now())
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
		names = args[1:]
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
	w := newTable(cmd.OutOrStdout())
	fmt.Fprintln(w, "HOST\tTUNNEL\tTYPE\tLISTEN\tTARGET\tPID\tSTARTED\tLOG")
	for _, entry := range st.Tunnels {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
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
	return w.Flush()
}

func newTable(out io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
}

func filterHostsByTag(hosts []model.Host, tag string) []model.Host {
	var out []model.Host
	for _, host := range hosts {
		for _, value := range host.Tags {
			if value == tag {
				out = append(out, host)
				break
			}
		}
	}
	return out
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
