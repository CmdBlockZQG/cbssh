package tui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cmdblock/cbssh/internal/config"
	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/sshclient"
	"github.com/cmdblock/cbssh/internal/state"
	"github.com/cmdblock/cbssh/internal/tunnel"
)

var errCanceled = errors.New("canceled")

var lastError string

const (
	styleBold  = "\033[1m"
	styleGreen = "\033[32m"
	styleRed   = "\033[31m"
	styleCyan  = "\033[36m"
	styleDim   = "\033[2m"
	styleReset = "\033[0m"
)

type menuCommand struct {
	Action string
	Args   []string
}

func Run(ctx context.Context, configPath string, statePath string) error {
	reader := bufio.NewReader(os.Stdin)
	defer func() { stdinClosed = true }()
	var lastCfg model.Config
	sortRecent := true
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			if lastCfg.Hosts == nil {
				return err
			}
			cfg = lastCfg
			fmt.Printf("%sWarning: config load failed, using cached: %v%s\n", styleRed, err, styleReset)
		} else {
			lastCfg = cfg
		}
		st, _, err := tunnel.Status(statePath, "")
		if err != nil {
			fmt.Printf("%sWarning: tunnel status error: %v%s\n", styleRed, err, styleReset)
		}
		clearScreen()
		printDashboard(configPath, cfg, st, sortRecent)
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
			printMainHelp()
			waitEnter(reader)
			continue
		case "q":
			return nil
		case "a":
			err = addHost(reader, configPath, cfg)
		case "e":
			err = editHost(reader, configPath, cfg, firstArg(choice.Args))
		case "d":
			err = deleteHost(reader, configPath, cfg, firstArg(choice.Args))
		case "t":
			err = manageTunnels(ctx, reader, configPath, statePath, cfg, firstArg(choice.Args))
		case "i":
			err = showHost(reader, statePath, cfg, firstArg(choice.Args))
		case "s":
			err = startTunnels(ctx, reader, statePath, configPath, cfg, choice.Args)
		case "x":
			err = stopTunnels(ctx, reader, statePath, cfg, choice.Args)
		case "c":
			err = connectHost(ctx, reader, configPath, statePath, cfg, firstArg(choice.Args))
		case "v":
			err = config.Validate(cfg)
		case "r":
			sortRecent = !sortRecent
			continue
		default:
			err = fmt.Errorf("unknown command %q, press ? for help", choice.Action)
		}
		if err != nil {
			if errors.Is(err, errCanceled) {
				continue
			}
			lastError = err.Error()
		}
	}
}

func printDashboard(configPath string, cfg model.Config, st model.State, sortRecent bool) {
	active := map[string]int{}
	for _, entry := range st.Tunnels {
		active[entry.HostName]++
	}
	hosts := append([]model.Host(nil), cfg.Hosts...)
	if sortRecent {
		sort.SliceStable(hosts, func(i, j int) bool {
			return st.Hosts[hosts[i].Name].LastUsed.After(st.Hosts[hosts[j].Name].LastUsed)
		})
	} else {
		sort.SliceStable(hosts, func(i, j int) bool { return hosts[i].Name < hosts[j].Name })
	}
	sortLabel := "recent"
	if !sortRecent {
		sortLabel = "name"
	}
	fmt.Printf("%s%s cbssh%s\n", styleCyan, styleBold, styleReset)
	if lastError != "" {
		fmt.Printf("%s%s%s\n", styleRed+styleBold, lastError, styleReset)
		lastError = ""
		fmt.Println(strings.Repeat("─", 80))
	} else {
		fmt.Println(strings.Repeat("─", 80))
	}
	fmt.Printf("Config: %s%s%s", styleDim, configPath, styleReset)
	fmt.Printf("  Hosts: %d  Active Tunnels: %d  %ssort: %s%s\n\n", len(cfg.Hosts), len(st.Tunnels), styleDim, sortLabel, styleReset)
	if len(hosts) == 0 {
		fmt.Println("No hosts configured.")
	} else {
		fmt.Printf("%s%-4s %-16s %-21s %-10s %-5s %-5s%s\n", styleBold, "NO", "NAME", "HOST", "USER", "TUN", "ACT", styleReset)
		for i, host := range hosts {
			count := active[host.Name]
			countStr := strconv.Itoa(count)
			if count > 0 {
				countStr = styleGreen + countStr + styleReset
			}
			fmt.Printf(" %-3d %-16s %-21s %-10s %-5d %-5s\n",
				i+1,
				host.Name,
				host.Address(),
				host.User,
				len(host.Tunnels),
				countStr,
			)
		}
	}
	if len(st.Tunnels) > 0 {
		fmt.Println()
		fmt.Printf("%s%-16s %-16s %-7s %-21s %-7s%s\n", styleBold, "ACTIVE HOST", "TUNNEL", "TYPE", "LISTEN", "PID", styleReset)
		for _, entry := range st.Tunnels {
			fmt.Printf("%s%-16s %-16s %-7s %-21s %-7d%s\n",
				styleGreen,
				entry.HostName,
				entry.TunnelName,
				entry.Type,
				entry.ListenAddress(),
				entry.PID,
				styleReset,
			)
		}
	}
	fmt.Println()
	fmt.Printf("  %s[c]%s connect  %s[a]%s add  %s[e]%s edit  %s[d]%s delete  %s[t]%s tunnels  %s[i]%s info\n",
		styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset)
	fmt.Printf("  %s[s]%s start  %s[x]%s stop  %s[r]%s sort  %s[v]%s validate  %s[?]%s help  %s[q]%s quit\n",
		styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset)
}

func printMainHelp() {
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Printf("  %sc <host>%s               connect to host\n", styleBold, styleReset)
	fmt.Printf("  %sa%s                      add host\n", styleBold, styleReset)
	fmt.Printf("  %se <host>%s               edit host\n", styleBold, styleReset)
	fmt.Printf("  %sd <host>%s               delete host\n", styleBold, styleReset)
	fmt.Printf("  %st <host>%s               tunnel menu\n", styleBold, styleReset)
	fmt.Printf("  %si <host>%s               show host info\n", styleBold, styleReset)
	fmt.Printf("  %ss <host> [<tun>..]%s     start tunnels\n", styleBold, styleReset)
	fmt.Printf("  %sx [<host> [<tun>..]]%s   stop tunnels\n", styleBold, styleReset)
	fmt.Printf("  %sr%s                      toggle sort (recent / name)\n", styleBold, styleReset)
	fmt.Printf("  %sv%s                      validate config\n", styleBold, styleReset)
	fmt.Printf("  %s?%s                      help\n", styleBold, styleReset)
	fmt.Printf("  %sq%s                      quit\n", styleBold, styleReset)
	fmt.Println()
	fmt.Printf("  %s<host>%s = host name or number\n", styleBold, styleReset)
	fmt.Printf("  %s<tun>%s = tunnel name or number, comma/space separated\n", styleBold, styleReset)
	fmt.Println("  All commands prompt interactively when arguments are omitted.")
}

func addHost(reader *bufio.Reader, configPath string, cfg model.Config) error {
	host := model.Host{}
	host.Name = promptRequiredString(reader, "Name", "")
	host.Host = promptRequiredString(reader, "Host", "")
	host.Port = promptPort(reader, "Port", 22)
	host.User = promptRequiredString(reader, "User", os.Getenv("USER"))
	host.Jump = promptString(reader, "Jump host name", "")
	host.Auth.Type = promptAuthType(reader, model.AuthTypeKey)
	if host.Auth.Type == model.AuthTypePassword {
		host.Auth.Password = promptRequiredString(reader, "Password", "")
	} else {
		host.Auth.KeyPath = promptRequiredString(reader, "Key path", cfg.DefaultKeyPath)
		host.Auth.Passphrase = promptString(reader, "Key passphrase", "")
		host.Auth.UseAgent = promptBool(reader, "Use ssh-agent", false)
	}
	cfg.Hosts = append(cfg.Hosts, host)
	return config.Save(configPath, cfg)
}

func editHost(reader *bufio.Reader, configPath string, cfg model.Config, selector string) error {
	index, err := selectHost(reader, cfg, selector)
	if err != nil {
		return err
	}
	host := cfg.Hosts[index]
	host.Name = promptRequiredString(reader, "Name", host.Name)
	host.Host = promptRequiredString(reader, "Host", host.Host)
	host.Port = promptPort(reader, "Port", host.Port)
	host.User = promptRequiredString(reader, "User", host.User)
	host.Jump = promptString(reader, "Jump host name", host.Jump)
	host.Auth.Type = promptAuthType(reader, host.Auth.Type)
	if host.Auth.Type == model.AuthTypePassword {
		if host.Auth.Password == "" {
			host.Auth.Password = promptRequiredString(reader, "Password", "")
		} else {
			next := promptString(reader, "Password (blank keeps current)", "")
			if next != "" {
				host.Auth.Password = next
			}
		}
		host.Auth.KeyPath = ""
		host.Auth.Passphrase = ""
		host.Auth.UseAgent = false
	} else {
		host.Auth.KeyPath = promptRequiredString(reader, "Key path", host.Auth.KeyPath)
		host.Auth.Passphrase = promptString(reader, "Key passphrase", host.Auth.Passphrase)
		host.Auth.UseAgent = promptBool(reader, "Use ssh-agent", host.Auth.UseAgent)
		host.Auth.Password = ""
	}
	cfg.Hosts[index] = host
	return config.Save(configPath, cfg)
}

func deleteHost(reader *bufio.Reader, configPath string, cfg model.Config, selector string) error {
	index, err := selectHost(reader, cfg, selector)
	if err != nil {
		return err
	}
	host := cfg.Hosts[index]
	if !promptBool(reader, "Delete "+host.Name, false) {
		return nil
	}
	cfg.Hosts = append(cfg.Hosts[:index], cfg.Hosts[index+1:]...)
	return config.Save(configPath, cfg)
}

func manageTunnels(ctx context.Context, reader *bufio.Reader, configPath string, statePath string, cfg model.Config, selector string) error {
	index, err := selectHost(reader, cfg, selector)
	if err != nil {
		return err
	}
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
		if index >= len(cfg.Hosts) {
			return fmt.Errorf("host index is no longer valid")
		}
		host := cfg.Hosts[index]
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
		fmt.Println(strings.Repeat("─", 80))
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
			printTunnelHelp(host.Name)
			waitEnter(reader)
			continue
		case "b":
			return nil
		case "a":
			err = addTunnel(reader, configPath, cfg, index)
		case "e":
			err = editTunnel(reader, configPath, cfg, index, firstArg(choice.Args))
		case "d":
			err = deleteTunnel(reader, configPath, cfg, index, firstArg(choice.Args))
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

func printTunnelHelp(hostName string) {
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

func connectHost(ctx context.Context, reader *bufio.Reader, configPath string, statePath string, cfg model.Config, selector string) error {
	index, err := selectHost(reader, cfg, selector)
	if err != nil {
		return err
	}
	host := cfg.Hosts[index]
	chain, err := config.ResolveChain(cfg, host.Name)
	if err != nil {
		return err
	}
	_ = state.MarkHostUsed(statePath, host.Name, time.Now())
	if err := sshclient.RunInteractive(ctx, cfg, chain); err != nil {
		fmt.Printf("\n%sSSH error: %v%s\n", styleRed+styleBold, err, styleReset)
	}
	os.Exit(0)
	return nil
}

func showHost(reader *bufio.Reader, statePath string, cfg model.Config, selector string) error {
	index, err := selectHost(reader, cfg, selector)
	if err != nil {
		return err
	}
	host := cfg.Hosts[index]
	chain, err := config.ResolveJumpNames(cfg, host.Name)
	if err != nil {
		return err
	}
	st, _, err := tunnel.Status(statePath, host.Name)
	if err != nil {
		return err
	}
	active := map[string]model.TunnelRuntime{}
	for _, entry := range st.Tunnels {
		active[entry.TunnelName] = entry
	}

	fmt.Println()
	fmt.Printf("%s%s%s\n", styleBold, host.Name, styleReset)
	fmt.Println(strings.Repeat("─", 40))
	fmt.Printf("Host: %s\n", host.Address())
	fmt.Printf("User: %s\n", host.User)
	jump := strings.Join(chain, " -> ")
	fmt.Printf("Jump: %s\n", emptyDash(jump))
	authLine := host.Auth.Type
	if host.Auth.Type == model.AuthTypeKey {
		authLine += " " + host.Auth.KeyPath
	}
	fmt.Printf("Auth: %s\n", authLine)
	fmt.Println()
	if len(host.Tunnels) == 0 {
		fmt.Println("Tunnels: none")
	} else {
		fmt.Printf("%s%-4s %-16s %-7s %-21s %-21s %-3s %-7s%s\n",
			styleBold, "NO", "NAME", "TYPE", "LISTEN", "TARGET", "DEF", "PID", styleReset)
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
				i+1, tun.Name, tun.Type,
				tun.ListenAddress(), emptyDash(tun.TargetAddress()),
				def, pid)
		}
	}
	waitEnter(reader)
	return nil
}

func selectHost(reader *bufio.Reader, cfg model.Config, selector string) (int, error) {
	if len(cfg.Hosts) == 0 {
		return 0, fmt.Errorf("no hosts configured")
	}
	if selector == "" {
		selector = promptString(reader, "Host number or name (blank to cancel)", "")
	}
	if selector == "" {
		return 0, errCanceled
	}
	return resolveHostSelector(cfg, selector)
}

func resolveHostSelector(cfg model.Config, value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errCanceled
	}
	if number, err := strconv.Atoi(value); err == nil {
		if number < 1 || number > len(cfg.Hosts) {
			return 0, fmt.Errorf("host number %d out of range", number)
		}
		return number - 1, nil
	}
	if index, ok, ambiguous := findNamedHost(cfg, func(name string) bool { return name == value }); ok {
		return index, nil
	} else if ambiguous {
		return 0, fmt.Errorf("host selector %q is ambiguous", value)
	}
	if index, ok, ambiguous := findNamedHost(cfg, func(name string) bool { return strings.EqualFold(name, value) }); ok {
		return index, nil
	} else if ambiguous {
		return 0, fmt.Errorf("host selector %q is ambiguous", value)
	}
	if index, ok, ambiguous := findNamedHost(cfg, func(name string) bool { return strings.HasPrefix(strings.ToLower(name), strings.ToLower(value)) }); ok {
		return index, nil
	} else if ambiguous {
		return 0, fmt.Errorf("host selector %q is ambiguous", value)
	}
	return 0, fmt.Errorf("host %q not found", value)
}

func selectTunnel(reader *bufio.Reader, host model.Host, selector string) (int, error) {
	if len(host.Tunnels) == 0 {
		return 0, fmt.Errorf("no tunnels configured")
	}
	if selector == "" {
		selector = promptString(reader, "Tunnel number or name (blank to cancel)", "")
	}
	if selector == "" {
		return 0, errCanceled
	}
	return resolveTunnelSelector(host, selector)
}

func resolveTunnelSelector(host model.Host, value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errCanceled
	}
	if number, err := strconv.Atoi(value); err == nil {
		if number < 1 || number > len(host.Tunnels) {
			return 0, fmt.Errorf("tunnel number %d out of range", number)
		}
		return number - 1, nil
	}
	if index, ok, ambiguous := findNamedTunnel(host, func(name string) bool { return name == value }); ok {
		return index, nil
	} else if ambiguous {
		return 0, fmt.Errorf("tunnel selector %q is ambiguous", value)
	}
	if index, ok, ambiguous := findNamedTunnel(host, func(name string) bool { return strings.EqualFold(name, value) }); ok {
		return index, nil
	} else if ambiguous {
		return 0, fmt.Errorf("tunnel selector %q is ambiguous", value)
	}
	if index, ok, ambiguous := findNamedTunnel(host, func(name string) bool { return strings.HasPrefix(strings.ToLower(name), strings.ToLower(value)) }); ok {
		return index, nil
	} else if ambiguous {
		return 0, fmt.Errorf("tunnel selector %q is ambiguous", value)
	}
	return 0, fmt.Errorf("tunnel %q not found", value)
}

func findNamedHost(cfg model.Config, matches func(string) bool) (int, bool, bool) {
	index := -1
	for i, host := range cfg.Hosts {
		if !matches(host.Name) {
			continue
		}
		if index != -1 {
			return 0, false, true
		}
		index = i
	}
	return index, index != -1, false
}

func findNamedTunnel(host model.Host, matches func(string) bool) (int, bool, bool) {
	index := -1
	for i, tun := range host.Tunnels {
		if !matches(tun.Name) {
			continue
		}
		if index != -1 {
			return 0, false, true
		}
		index = i
	}
	return index, index != -1, false
}

func selectHostAndTunnels(reader *bufio.Reader, cfg model.Config, args []string, tunnelPrompt string) (model.Host, []string, error) {
	var selector string
	if len(args) > 0 {
		selector = args[0]
	} else {
		selector = promptString(reader, "Host number or name (blank to cancel)", "")
	}
	index, err := resolveHostSelector(cfg, selector)
	if err != nil {
		return model.Host{}, nil, err
	}
	host := cfg.Hosts[index]
	var rawNames []string
	if len(args) > 1 {
		rawNames = args[1:]
	} else {
		rawNames = splitArgs(promptString(reader, tunnelPrompt, ""))
	}
	names, err := resolveTunnelSelectors(host, rawNames)
	if err != nil {
		return model.Host{}, nil, err
	}
	return host, names, nil
}

func resolveTunnelSelectors(host model.Host, selectors []string) ([]string, error) {
	selectors = splitArgs(strings.Join(selectors, " "))
	if len(selectors) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(selectors))
	seen := map[string]bool{}
	for _, selector := range selectors {
		index, err := resolveTunnelSelector(host, selector)
		if err != nil {
			return nil, err
		}
		name := host.Tunnels[index].Name
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names, nil
}

func activeTunnelMap(st model.State) map[string]model.TunnelRuntime {
	active := map[string]model.TunnelRuntime{}
	for _, entry := range st.Tunnels {
		active[entry.TunnelName] = entry
	}
	return active
}

func promptString(reader *bufio.Reader, label string, fallback string) string {
	if fallback == "" {
		fmt.Printf("%s: ", label)
	} else {
		fmt.Printf("%s [%s]: ", label, fallback)
	}
	value, _ := readLine(reader)
	if value == "" {
		return fallback
	}
	return value
}

func promptRequiredString(reader *bufio.Reader, label string, fallback string) string {
	for {
		if stdinClosed {
			return ""
		}
		value := promptString(reader, label, fallback)
		if value != "" {
			return value
		}
		fmt.Printf("%s is required.\n", label)
	}
}

func promptPort(reader *bufio.Reader, label string, fallback int) int {
	for {
		if stdinClosed {
			return 0
		}
		var value string
		if fallback > 0 {
			value = promptString(reader, label, strconv.Itoa(fallback))
		} else {
			value = promptString(reader, label, "")
		}
		if value == "" {
			fmt.Println("Please enter a port.")
			continue
		}
		number, err := strconv.Atoi(value)
		if err != nil {
			fmt.Println("Please enter a number.")
			continue
		}
		if number < 1 || number > 65535 {
			fmt.Println("Port must be between 1 and 65535.")
			continue
		}
		return number
	}
}

func promptAuthType(reader *bufio.Reader, fallback string) string {
	for {
		if stdinClosed {
			return model.AuthTypeKey
		}
		value := strings.ToLower(promptString(reader, "Auth type (key/password)", fallback))
		switch value {
		case "k", "key":
			return model.AuthTypeKey
		case "p", "pass", "password":
			return model.AuthTypePassword
		default:
			fmt.Println("Please enter key or password.")
		}
	}
}

func promptTunnelType(reader *bufio.Reader, fallback string) string {
	for {
		if stdinClosed {
			return model.TunnelTypeLocal
		}
		value := strings.ToLower(promptString(reader, "Type (local/remote/dynamic)", fallback))
		switch value {
		case "l", "local":
			return model.TunnelTypeLocal
		case "r", "remote":
			return model.TunnelTypeRemote
		case "d", "dynamic":
			return model.TunnelTypeDynamic
		default:
			fmt.Println("Please enter local, remote, or dynamic.")
		}
	}
}

func promptBool(reader *bufio.Reader, label string, fallback bool) bool {
	defaultValue := "n"
	if fallback {
		defaultValue = "y"
	}
	for {
		if stdinClosed {
			return fallback
		}
		value := strings.ToLower(promptString(reader, label+" (y/n)", defaultValue))
		switch value {
		case "y", "yes", "true":
			return true
		case "n", "no", "false":
			return false
		default:
			fmt.Println("Please enter y or n.")
		}
	}
}

var stdinClosed bool

func readChoice(reader *bufio.Reader, label string) (string, error) {
	fmt.Printf("%s: ", label)
	return readLine(reader)
}

func readLine(reader *bufio.Reader) (string, error) {
	value, err := reader.ReadString('\n')
	if err == io.EOF {
		if strings.TrimSpace(value) == "" {
			stdinClosed = true
		}
		return strings.TrimSpace(value), nil
	}
	return strings.TrimSpace(value), err
}

func waitEnter(reader *bufio.Reader) {
	fmt.Print("Press Enter to continue...")
	_, _ = reader.ReadString('\n')
}

func splitArgs(value string) []string {
	value = strings.ReplaceAll(value, ",", " ")
	return strings.Fields(value)
}

func parseCommand(value string) menuCommand {
	fields := splitArgs(value)
	if len(fields) == 0 {
		return menuCommand{}
	}
	return menuCommand{
		Action: strings.ToLower(fields[0]),
		Args:   fields[1:],
	}
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
