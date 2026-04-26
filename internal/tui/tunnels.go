package tui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/cmdblock/cbssh/internal/config"
	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/tunnel"
)

func manageTunnels(ctx context.Context, reader *bufio.Reader, configPath string, statePath string, cfg model.Config, selector string) error {
	index, err := selectHost(reader, cfg, selector)
	if err != nil {
		return err
	}
	hostName := cfg.Hosts[index].Name

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		newCfg, loadErr := config.Load(configPath)
		if loadErr != nil {
			fmt.Printf("%sWarning: config load failed, using cached: %v%s\n", styleRed, loadErr, styleReset)
		} else {
			cfg = newCfg
		}
		// The menu is anchored by host name so external config reordering does
		// not make tunnel edits apply to the wrong host index.
		hostIndex, ok := hostIndexByName(cfg, hostName)
		if !ok {
			return fmt.Errorf("host %q is no longer configured", hostName)
		}
		host := cfg.Hosts[hostIndex]
		st, _, err := tunnel.Status(statePath, host.Name)
		if err != nil {
			fmt.Printf("%sWarning: tunnel status error: %v%s\n", styleRed, err, styleReset)
		}
		active := activeTunnelMap(st)
		clearScreen()
		fmt.Printf("%sTunnels for %s%s\n", styleBold+styleCyan, host.Name, styleReset)
		if lastError != "" {
			fmt.Printf("%s%s%s\n", styleRed+styleBold, lastError, styleReset)
			lastError = ""
		}
		fmt.Println(strings.Repeat("-", 80))
		if len(host.Tunnels) == 0 {
			fmt.Println("No tunnels configured.")
		} else {
			fmt.Printf("%s%-4s %-16s %-7s %-21s %-21s %-3s %-7s%s\n", styleBold, "NO", "NAME", "TYPE", "LISTEN", "TARGET", "DEF", "PID", styleReset)
			for i, tun := range host.Tunnels {
				_, isActive := active[tun.Name]
				pid := "-"
				if isActive {
					pid = strconv.Itoa(active[tun.Name].PID)
				}
				def := 0
				if tun.Default {
					def = 1
				}
				fmt.Printf(" %-3d %-16s %-7s %-21s %-21s %-3d %-7s\n",
					i+1,
					tun.Name,
					tun.Type,
					tun.ListenAddress(),
					emptyDash(tun.TargetAddress()),
					def,
					pid,
				)
			}
		}
		fmt.Println()
		fmt.Printf("  %s[a]%s add  %s[e]%s edit  %s[d]%s delete  %s[s]%s start  %s[x]%s stop  %s[?]%s help  %s[b]%s back\n",
			styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset)
		rawInput, readErr := readChoice(reader, "Action")
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
		if stdinClosed {
			return nil
		}
		choice := parseCommand(rawInput)
		switch choice.Action {
		case "":
			continue
		case "?":
			printTunnelHelp()
			waitEnter(reader)
			continue
		case "b":
			return nil
		case "a":
			err = addTunnel(reader, configPath, cfg, hostIndex)
		case "e":
			err = editTunnel(reader, configPath, cfg, hostIndex, firstArg(choice.Args))
		case "d":
			err = deleteTunnel(reader, configPath, cfg, hostIndex, firstArg(choice.Args))
		case "s":
			if len(choice.Args) > 0 {
				err = startHostTunnels(ctx, statePath, configPath, host, choice.Args)
			} else {
				raw := splitArgs(promptString(reader, "Tunnel names (blank for defaults)", ""))
				err = startHostTunnels(ctx, statePath, configPath, host, raw)
			}
		case "x":
			if len(choice.Args) > 0 {
				err = stopHostTunnels(ctx, statePath, host, choice.Args)
			} else {
				raw := splitArgs(promptString(reader, "Tunnel names (blank to stop all)", ""))
				err = stopHostTunnels(ctx, statePath, host, raw)
			}
		default:
			err = fmt.Errorf("unknown action")
		}
		if err != nil {
			if errors.Is(err, errCanceled) {
				continue
			}
			lastError = err.Error()
		}
	}
}

func printTunnelHelp() {
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Printf("  %ss [<tun>..]%s   start tunnels\n", styleBold, styleReset)
	fmt.Printf("  %sx [<tun>..]%s   stop tunnels\n", styleBold, styleReset)
	fmt.Printf("  %sa%s             add a new tunnel\n", styleBold, styleReset)
	fmt.Printf("  %se <tun>%s       edit tunnel <tun>\n", styleBold, styleReset)
	fmt.Printf("  %sd <tun>%s       delete tunnel <tun>\n", styleBold, styleReset)
	fmt.Printf("  %s?%s             show this help\n", styleBold, styleReset)
	fmt.Printf("  %sb%s             back to main menu\n", styleBold, styleReset)
	fmt.Println()
	fmt.Printf("  %s<tun>%s = tunnel name or number, comma/space separated\n", styleBold, styleReset)
	fmt.Println("  All commands prompt interactively when arguments are omitted.")
}

func addTunnel(reader *bufio.Reader, configPath string, cfg model.Config, hostIndex int) error {
	tun := promptTunnel(reader, model.Tunnel{Type: model.TunnelTypeLocal, ListenHost: "127.0.0.1", TargetHost: "127.0.0.1"})
	cfg.Hosts[hostIndex].Tunnels = append(cfg.Hosts[hostIndex].Tunnels, tun)
	return config.Save(configPath, cfg)
}

func editTunnel(reader *bufio.Reader, configPath string, cfg model.Config, hostIndex int, selector string) error {
	tunnelIndex, err := selectTunnel(reader, cfg.Hosts[hostIndex], selector)
	if err != nil {
		return err
	}
	cfg.Hosts[hostIndex].Tunnels[tunnelIndex] = promptTunnel(reader, cfg.Hosts[hostIndex].Tunnels[tunnelIndex])
	return config.Save(configPath, cfg)
}

func deleteTunnel(reader *bufio.Reader, configPath string, cfg model.Config, hostIndex int, selector string) error {
	tunnelIndex, err := selectTunnel(reader, cfg.Hosts[hostIndex], selector)
	if err != nil {
		return err
	}
	tun := cfg.Hosts[hostIndex].Tunnels[tunnelIndex]
	if !promptBool(reader, "Delete "+tun.Name, false) {
		return nil
	}
	tunnels := cfg.Hosts[hostIndex].Tunnels
	cfg.Hosts[hostIndex].Tunnels = append(tunnels[:tunnelIndex], tunnels[tunnelIndex+1:]...)
	return config.Save(configPath, cfg)
}

func promptTunnel(reader *bufio.Reader, tun model.Tunnel) model.Tunnel {
	tun.Name = promptRequiredString(reader, "Name", tun.Name)
	tun.Type = promptTunnelType(reader, tun.Type)
	tun.ListenHost = promptString(reader, "Listen host", tun.ListenHost)
	tun.ListenPort = promptPort(reader, "Listen port", tun.ListenPort)
	if tun.Type == model.TunnelTypeDynamic {
		tun.TargetHost = ""
		tun.TargetPort = 0
	} else {
		tun.TargetHost = promptRequiredString(reader, "Target host", tun.TargetHost)
		tun.TargetPort = promptPort(reader, "Target port", tun.TargetPort)
	}
	tun.Default = promptBool(reader, "Default", tun.Default)
	return tun
}
