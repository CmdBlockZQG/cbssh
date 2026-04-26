package tui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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
		printDashboard(configPath, cfg, st)
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
		case "?", "h", "help":
			printMainHelp()
			waitEnter(reader)
			continue
		case "q", "quit", "exit":
			return nil
		case "a", "add":
			err = addHost(reader, configPath, cfg)
		case "e", "edit":
			err = editHost(reader, configPath, cfg, "")
		case "d", "delete":
			err = deleteHost(reader, configPath, cfg, "")
		case "t", "tunnels":
			err = manageTunnels(ctx, reader, configPath, statePath, cfg, "")
		case "s", "start":
			err = startTunnels(ctx, reader, statePath, configPath, cfg, nil)
		case "x", "stop":
			err = stopTunnels(ctx, reader, statePath, cfg, nil)
		case "c", "connect":
			err = connectHost(ctx, reader, configPath, statePath, cfg, "")
		case "v", "validate":
			err = config.Validate(cfg)
		default:
			err = connectHost(ctx, reader, configPath, statePath, cfg, choice.Action)
		}
		if err != nil {
			if errors.Is(err, errCanceled) {
				continue
			}
			lastError = err.Error()
		}
	}
}

func printDashboard(configPath string, cfg model.Config, st model.State) {
	active := map[string]int{}
	for _, entry := range st.Tunnels {
		active[entry.HostName]++
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
	fmt.Printf("  Hosts: %d  Active: %d\n\n", len(cfg.Hosts), len(st.Tunnels))
	if len(cfg.Hosts) == 0 {
		fmt.Println("No hosts configured.")
	} else {
		fmt.Printf("%s%-4s %-22s %-24s %-14s %-8s %-8s%s\n", styleBold, "NO", "NAME", "HOST", "USER", "TUN", "ACT", styleReset)
		for i, host := range cfg.Hosts {
			count := active[host.Name]
			countStr := strconv.Itoa(count)
			if count > 0 {
				countStr = styleGreen + countStr + styleReset
			}
			fmt.Printf(" %-3d %-22s %-24s %-14s %-8d %-8s\n",
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
		fmt.Printf("%s%-22s %-18s %-9s %-22s %-8s%s\n", styleBold, "ACTIVE HOST", "TUNNEL", "TYPE", "LISTEN", "PID", styleReset)
		for _, entry := range st.Tunnels {
			fmt.Printf("%s%-22s %-18s %-9s %-22s %-8d%s\n",
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
	fmt.Printf("  %s[c]%s connect  %s[a]%s add  %s[e]%s edit  %s[d]%s delete  %s[m]%s tunnels\n",
		styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset)
	fmt.Printf("  %s[s]%s start  %s[x]%s stop  %s[v]%s validate  %s[?]%s help  %s[q]%s quit\n",
		styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset)
}

func printMainHelp() {
	fmt.Println()
	fmt.Println("Single-letter commands (all parameters via wizard):")
	fmt.Printf("  %s[c]%s or %s<name>%s   connect to host\n", styleBold, styleReset, styleBold, styleReset)
	fmt.Printf("  %s[a]%s           add host\n", styleBold, styleReset)
	fmt.Printf("  %s[e]%s           edit host (wizard asks which)\n", styleBold, styleReset)
	fmt.Printf("  %s[d]%s           delete host (wizard asks which)\n", styleBold, styleReset)
	fmt.Printf("  %s[m]%s           manage tunnels for a host\n", styleBold, styleReset)
	fmt.Printf("  %s[s]%s           start tunnels (wizard asks host + names)\n", styleBold, styleReset)
	fmt.Printf("  %s[x]%s           stop tunnels (wizard asks host + names)\n", styleBold, styleReset)
	fmt.Printf("  %s[v]%s           validate config\n", styleBold, styleReset)
	fmt.Printf("  %s[?]%s           help\n", styleBold, styleReset)
	fmt.Printf("  %s[q]%s           quit\n", styleBold, styleReset)
}

func addHost(reader *bufio.Reader, configPath string, cfg model.Config) error {
	host := model.Host{}
	host.Name = promptRequiredString(reader, "Name", "")
	host.Host = promptRequiredString(reader, "Host", "")
	host.Port = promptPort(reader, "Port", 22)
	host.User = promptRequiredString(reader, "User", os.Getenv("USER"))
	host.Jump = promptString(reader, "Jump host name", "")
	host.Tags = splitCSV(promptString(reader, "Tags", ""))
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
	host.Tags = splitCSV(promptString(reader, "Tags", strings.Join(host.Tags, ",")))
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
			fmt.Printf("%s%-4s %-18s %-9s %-22s %-22s %-8s %-8s%s\n", styleBold, "NO", "NAME", "TYPE", "LISTEN", "TARGET", "DEF", "ACT", styleReset)
			for i, tun := range host.Tunnels {
				_, isActive := active[tun.Name]
				activeMark := " " + styleGreen + "●" + styleReset
				if !isActive {
					activeMark = " " + styleDim + "○" + styleReset
				}
				fmt.Printf(" %-3d %-18s %-9s %-22s %-22s %-8t%s\n",
					i+1,
					tun.Name,
					tun.Type,
					tun.ListenAddress(),
					emptyDash(tun.TargetAddress()),
					tun.Default,
					activeMark,
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
		case "?", "h", "help":
			printTunnelHelp(host.Name)
			waitEnter(reader)
			continue
		case "b", "back":
			return nil
		case "a", "add":
			err = addTunnel(reader, configPath, cfg, index)
		case "e", "edit":
			err = editTunnel(reader, configPath, cfg, index, "")
		case "d", "delete":
			err = deleteTunnel(reader, configPath, cfg, index, "")
		case "s", "start":
			raw := splitArgs(promptString(reader, "Tunnel names (blank for defaults)", ""))
			err = startHostTunnels(ctx, statePath, configPath, host, raw)
		case "x", "stop":
			raw := splitArgs(promptString(reader, "Tunnel names (blank to stop all)", ""))
			err = stopHostTunnels(ctx, statePath, host, raw)
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
	fmt.Println("Single-letter commands (all parameters via wizard):")
	fmt.Printf("  %s[s]%s    start tunnels (blank = defaults)\n", styleBold, styleReset)
	fmt.Printf("  %s[x]%s    stop tunnels (blank = stop all)\n", styleBold, styleReset)
	fmt.Printf("  %s[a]%s    add tunnel\n", styleBold, styleReset)
	fmt.Printf("  %s[e]%s    edit tunnel (wizard asks which)\n", styleBold, styleReset)
	fmt.Printf("  %s[d]%s    delete tunnel (wizard asks which)\n", styleBold, styleReset)
	fmt.Printf("  %s[b]%s    back to main menu\n", styleBold, styleReset)
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
		names, err = resolveTunnelSelectors(host, args[1:])
		if err != nil {
			return err
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
			rawNames := splitArgs(promptString(reader, "Tunnel names", ""))
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
	return sshclient.RunInteractive(ctx, cfg, chain)
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

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	var out []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
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
