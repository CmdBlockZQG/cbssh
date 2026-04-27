package cmd

import (
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/cmdblock/cbssh/internal/tunnel"
)

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
