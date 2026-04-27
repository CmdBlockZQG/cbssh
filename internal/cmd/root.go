package cmd

import (
	"github.com/spf13/cobra"

	"github.com/cmdblock/cbssh/internal/platform"
	"github.com/cmdblock/cbssh/internal/tui"
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
	root.AddCommand(a.newInfoCommand())
	root.AddCommand(a.newConnectCommand())
	root.AddCommand(a.newFileCommand())
	root.AddCommand(a.newUpCommand())
	root.AddCommand(a.newDownCommand())
	root.AddCommand(a.newBrowseCommand())
	root.AddCommand(a.newTunnelCommand())
	root.AddCommand(a.newStatusCommand())
	root.AddCommand(a.newStopCommand())
	root.AddCommand(a.newStartCommand())
	root.AddCommand(a.newRestartCommand())
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
