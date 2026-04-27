package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cmdblock/cbssh/internal/config"
	"github.com/cmdblock/cbssh/internal/hostview"
	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/sshclient"
	"github.com/cmdblock/cbssh/internal/state"
	"github.com/cmdblock/cbssh/internal/tunnel"
)

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
			hosts, err := hostview.Sort(cfg.Hosts, st, sortMode)
			if err != nil {
				return err
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
	c.Flags().StringVar(&sortMode, "sort", hostview.SortRecent, "Sort hosts by recent or name")
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
			fmt.Fprintf(out, "%s%-4s %-16s %-1s %-21s %-21s %-3s %-7s%s\n",
				cliBold, "NO", "NAME", "T", "LISTEN", "TARGET", "DEF", "PID", cliReset)
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
				fmt.Fprintf(out, " %-3d %-16s %-1s %-21s %-21s %-3d %-7s\n",
					i+1,
					tun.Name,
					model.TunnelTypeCode(tun.Type),
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
