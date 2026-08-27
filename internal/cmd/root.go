package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/cmdblock/cbssh/internal/config"
	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/openssh"
	"github.com/cmdblock/cbssh/internal/platform"
	"github.com/cmdblock/cbssh/internal/tunnel"
)

type app struct {
	configPath    string
	sshConfigPath string
	statePath     string
}

func NewRootCommand(version string) *cobra.Command {
	a := &app{}
	root := &cobra.Command{
		Use:           "cbssh",
		Short:         "Run named OpenSSH tunnels in the background",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.PersistentFlags().StringVar(&a.configPath, "config", platform.DefaultConfigPath(), "Path to tunnels.toml")
	root.PersistentFlags().StringVar(&a.sshConfigPath, "ssh-config", "", "Optional OpenSSH config passed with ssh -F")
	root.PersistentFlags().StringVar(&a.statePath, "state", platform.DefaultStatePath(), "Path to the runtime registry")
	_ = root.PersistentFlags().MarkHidden("state")

	root.AddCommand(a.newListCommand())
	root.AddCommand(a.newStartCommand())
	root.AddCommand(a.newStopCommand())
	root.AddCommand(a.newRestartCommand())
	root.AddCommand(a.newStatusCommand())
	root.AddCommand(a.newLogsCommand())
	root.AddCommand(a.newConfigCommand())
	return root
}

func (a *app) manager() *tunnel.Manager {
	return tunnel.NewManager(tunnel.Options{
		ConfigPath: a.configPath, SSHConfigPath: a.sshConfigPath, StatePath: a.statePath,
	})
}

func (a *app) newListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured tunnels",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(a.configPath)
			if err != nil {
				return err
			}
			writeTunnelList(cmd.OutOrStdout(), cfg.Tunnels)
			return nil
		},
	}
}

func (a *app) newStartCommand() *cobra.Command {
	var all bool
	command := &cobra.Command{
		Use:   "start <name...>",
		Short: "Start named tunnels",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(a.configPath)
			if err != nil {
				return err
			}
			selected, err := config.Select(cfg, args, all)
			if err != nil {
				return err
			}
			if len(selected) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No tunnels configured.")
				return nil
			}
			manager := a.manager()
			var failures []error
			for _, definition := range selected {
				result, err := manager.Start(cmd.Context(), definition)
				if err != nil {
					failures = append(failures, fmt.Errorf("%s: %w", definition.Name, err))
					continue
				}
				if result.Changed {
					fmt.Fprintf(cmd.OutOrStdout(), "Started %s via %s (master %s)\n", result.Name, result.Host, shortID(result.MasterID))
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "%s is already running.\n", result.Name)
				}
			}
			return errors.Join(failures...)
		},
	}
	command.Flags().BoolVar(&all, "all", false, "Start every configured tunnel")
	return command
}

func (a *app) newStopCommand() *cobra.Command {
	var all bool
	command := &cobra.Command{
		Use:   "stop <name...>",
		Short: "Stop named tunnels",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if all && len(args) > 0 {
				return errors.New("tunnel names and --all cannot be used together")
			}
			if !all && len(args) == 0 {
				return errors.New("provide at least one tunnel name or use --all")
			}
			if err := rejectDuplicateNames(args); err != nil {
				return err
			}
			manager := a.manager()
			names := args
			if all {
				var err error
				names, err = manager.ActiveNames(cmd.Context())
				if err != nil {
					return err
				}
			}
			if len(names) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No active tunnels.")
				return nil
			}
			var failures []error
			for _, name := range names {
				result, err := manager.Stop(cmd.Context(), name)
				if err != nil {
					failures = append(failures, fmt.Errorf("%s: %w", name, err))
					continue
				}
				if result.Changed {
					fmt.Fprintf(cmd.OutOrStdout(), "Stopped %s.\n", name)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "%s is already stopped.\n", name)
				}
			}
			return errors.Join(failures...)
		},
	}
	command.Flags().BoolVar(&all, "all", false, "Stop every active tunnel in this config namespace")
	return command
}

func (a *app) newRestartCommand() *cobra.Command {
	var all bool
	command := &cobra.Command{
		Use:   "restart <name...>",
		Short: "Restart named tunnels with their current definitions",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(a.configPath)
			if err != nil {
				return err
			}
			selected, err := config.Select(cfg, args, all)
			if err != nil {
				return err
			}
			if len(selected) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No tunnels configured.")
				return nil
			}
			manager := a.manager()
			var failures []error
			failedStops := make(map[string]bool)
			stopNames := make([]string, 0, len(selected))
			for _, definition := range selected {
				stopNames = append(stopNames, definition.Name)
			}
			if all {
				stopNames, err = manager.ActiveNames(cmd.Context())
				if err != nil {
					return err
				}
			}
			for _, name := range stopNames {
				if _, err := manager.Stop(cmd.Context(), name); err != nil {
					failedStops[name] = true
					failures = append(failures, fmt.Errorf("%s: %w", name, err))
				}
			}
			for _, definition := range selected {
				if failedStops[definition.Name] {
					continue
				}
				result, err := manager.Start(cmd.Context(), definition)
				if err != nil {
					failures = append(failures, fmt.Errorf("%s: %w", definition.Name, err))
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Restarted %s via %s (master %s)\n", result.Name, result.Host, shortID(result.MasterID))
			}
			return errors.Join(failures...)
		},
	}
	command.Flags().BoolVar(&all, "all", false, "Restart every configured tunnel")
	return command
}

func (a *app) newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status [name...]",
		Short: "Show configured and active tunnel status",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := rejectDuplicateNames(args); err != nil {
				return err
			}
			cfg, err := config.Load(a.configPath)
			if err != nil {
				return err
			}
			rows, err := a.manager().Status(cmd.Context(), cfg.Tunnels, args)
			if err != nil {
				return err
			}
			writeStatus(cmd.OutOrStdout(), rows)
			if len(args) > 0 {
				for _, row := range rows {
					if row.State != "running" {
						return errors.New("one or more requested tunnels are not running")
					}
				}
			}
			return nil
		},
	}
}

func (a *app) newLogsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logs <name>",
		Short: "Print the latest shared Master log for a tunnel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := a.manager().LogPath(args[0])
			if err != nil {
				return err
			}
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(cmd.OutOrStdout(), file)
			return err
		},
	}
}

func (a *app) newConfigCommand() *cobra.Command {
	command := &cobra.Command{Use: "config", Short: "Manage the tunnel manifest", RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() }}
	command.AddCommand(&cobra.Command{
		Use: "path", Short: "Print the tunnel manifest path", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), platform.ExpandPath(a.configPath))
			return nil
		},
	})
	command.AddCommand(&cobra.Command{
		Use: "init", Short: "Create an empty tunnel manifest", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.Init(a.configPath); err != nil {
				if errors.Is(err, config.ErrAlreadyExists) {
					fmt.Fprintf(cmd.OutOrStdout(), "Config already exists: %s\n", platform.ExpandPath(a.configPath))
					return nil
				}
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created %s\n", platform.ExpandPath(a.configPath))
			return nil
		},
	})
	command.AddCommand(&cobra.Command{
		Use: "validate", Short: "Validate the tunnel manifest and referenced SSH hosts", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(a.configPath)
			if err != nil {
				return err
			}
			runner := openssh.NewCommandRunner()
			hosts := make(map[string]struct{})
			for _, definition := range cfg.Tunnels {
				hosts[definition.Host] = struct{}{}
			}
			ordered := make([]string, 0, len(hosts))
			for host := range hosts {
				ordered = append(ordered, host)
			}
			sort.Strings(ordered)
			for _, host := range ordered {
				if err := runner.ValidateHost(cmd.Context(), host, platform.CanonicalPath(a.sshConfigPath)); err != nil {
					return err
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Valid tunnel config: %d tunnel(s), %d SSH host(s).\n", len(cfg.Tunnels), len(hosts))
			return nil
		},
	})
	return command
}

func writeTunnelList(output io.Writer, tunnels []model.Tunnel) {
	if len(tunnels) == 0 {
		fmt.Fprintln(output, "No tunnels configured.")
		return
	}
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "NAME\tTYPE\tHOST\tFORWARD")
	for _, definition := range tunnels {
		flag, spec, _ := openssh.ForwardArguments(definition)
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s %s\n", definition.Name, definition.Type, definition.Host, flag, spec)
	}
	_ = writer.Flush()
}

func writeStatus(output io.Writer, rows []tunnel.Status) {
	if len(rows) == 0 {
		fmt.Fprintln(output, "No tunnels configured or active.")
		return
	}
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "NAME\tSTATE\tHOST\tTYPE\tMASTER")
	for _, row := range rows {
		stateName := row.State
		if row.Orphaned {
			stateName += " (unconfigured)"
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", row.Name, stateName, row.Definition.Host, row.Definition.Type, shortID(row.MasterID))
	}
	_ = writer.Flush()
}

func rejectDuplicateNames(names []string) error {
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			return errors.New("tunnel name must not be empty")
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("duplicate tunnel name %q", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func shortID(id string) string {
	if id == "" {
		return "-"
	}
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
