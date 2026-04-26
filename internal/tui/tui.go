package tui

import (
	"bufio"
	"context"
	"fmt"
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

func Run(ctx context.Context, configPath string, statePath string) error {
	reader := bufio.NewReader(os.Stdin)
	for {
		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}
		st, _, err := tunnel.Status(statePath, "")
		if err != nil {
			return err
		}
		clearScreen()
		printDashboard(configPath, cfg, st)
		choice := readChoice(reader, "Action")
		switch strings.ToLower(choice) {
		case "q", "quit", "exit":
			return nil
		case "a", "add":
			err = addHost(reader, configPath, cfg)
		case "e", "edit":
			err = editHost(reader, configPath, cfg)
		case "d", "delete":
			err = deleteHost(reader, configPath, cfg)
		case "m", "tunnels":
			err = manageTunnels(ctx, reader, configPath, statePath, cfg)
		case "s", "start":
			err = startTunnels(ctx, reader, statePath, configPath)
		case "x", "stop":
			err = stopTunnels(ctx, reader, statePath)
		case "c", "connect":
			err = connectHost(ctx, reader, configPath, statePath, cfg)
		case "v", "validate":
			err = config.Validate(cfg)
		default:
			err = fmt.Errorf("unknown action %q", choice)
		}
		if err != nil {
			fmt.Printf("\nError: %v\n", err)
		} else {
			fmt.Println("\nDone.")
		}
		waitEnter(reader)
	}
}

func printDashboard(configPath string, cfg model.Config, st model.State) {
	active := map[string]int{}
	for _, entry := range st.Tunnels {
		active[entry.HostName]++
	}
	fmt.Println("cbssh")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Config: %s\n\n", configPath)
	if len(cfg.Hosts) == 0 {
		fmt.Println("No hosts configured.")
	} else {
		fmt.Printf("%-4s %-22s %-24s %-14s %-8s %-8s\n", "NO", "NAME", "HOST", "USER", "TUNNELS", "ACTIVE")
		for i, host := range cfg.Hosts {
			fmt.Printf("%-4d %-22s %-24s %-14s %-8d %-8d\n",
				i+1,
				host.Name,
				host.Address(),
				host.User,
				len(host.Tunnels),
				active[host.Name],
			)
		}
	}
	fmt.Println()
	fmt.Println("[a] Add  [e] Edit  [d] Delete  [m] Tunnels  [c] Connect")
	fmt.Println("[s] Start tunnels  [x] Stop tunnels  [v] Validate  [q] Quit")
}

func addHost(reader *bufio.Reader, configPath string, cfg model.Config) error {
	host := model.Host{}
	host.Name = promptString(reader, "Name", "")
	host.Host = promptString(reader, "Host", "")
	host.Port = promptInt(reader, "Port", 22)
	host.User = promptString(reader, "User", os.Getenv("USER"))
	host.Jump = promptString(reader, "Jump host name", "")
	host.Tags = splitCSV(promptString(reader, "Tags", ""))
	host.Auth.Type = promptString(reader, "Auth type", model.AuthTypeKey)
	if host.Auth.Type == model.AuthTypePassword {
		host.Auth.Password = promptString(reader, "Password", "")
	} else {
		host.Auth.KeyPath = promptString(reader, "Key path", cfg.DefaultKeyPath)
		host.Auth.Passphrase = promptString(reader, "Key passphrase", "")
		host.Auth.UseAgent = promptBool(reader, "Use ssh-agent", false)
	}
	cfg.Hosts = append(cfg.Hosts, host)
	return config.Save(configPath, cfg)
}

func editHost(reader *bufio.Reader, configPath string, cfg model.Config) error {
	index, err := selectHost(reader, cfg)
	if err != nil {
		return err
	}
	host := cfg.Hosts[index]
	host.Name = promptString(reader, "Name", host.Name)
	host.Host = promptString(reader, "Host", host.Host)
	host.Port = promptInt(reader, "Port", host.Port)
	host.User = promptString(reader, "User", host.User)
	host.Jump = promptString(reader, "Jump host name", host.Jump)
	host.Tags = splitCSV(promptString(reader, "Tags", strings.Join(host.Tags, ",")))
	host.Auth.Type = promptString(reader, "Auth type", host.Auth.Type)
	if host.Auth.Type == model.AuthTypePassword {
		next := promptString(reader, "Password (blank keeps current)", "")
		if next != "" {
			host.Auth.Password = next
		}
		host.Auth.KeyPath = ""
		host.Auth.Passphrase = ""
		host.Auth.UseAgent = false
	} else {
		host.Auth.KeyPath = promptString(reader, "Key path", host.Auth.KeyPath)
		host.Auth.Passphrase = promptString(reader, "Key passphrase", host.Auth.Passphrase)
		host.Auth.UseAgent = promptBool(reader, "Use ssh-agent", host.Auth.UseAgent)
		host.Auth.Password = ""
	}
	cfg.Hosts[index] = host
	return config.Save(configPath, cfg)
}

func deleteHost(reader *bufio.Reader, configPath string, cfg model.Config) error {
	index, err := selectHost(reader, cfg)
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

func manageTunnels(ctx context.Context, reader *bufio.Reader, configPath string, statePath string, cfg model.Config) error {
	index, err := selectHost(reader, cfg)
	if err != nil {
		return err
	}
	for {
		cfg, err = config.Load(configPath)
		if err != nil {
			return err
		}
		if index >= len(cfg.Hosts) {
			return fmt.Errorf("host index is no longer valid")
		}
		host := cfg.Hosts[index]
		clearScreen()
		fmt.Printf("Tunnels for %s\n", host.Name)
		fmt.Println(strings.Repeat("=", 80))
		if len(host.Tunnels) == 0 {
			fmt.Println("No tunnels configured.")
		} else {
			fmt.Printf("%-4s %-18s %-9s %-22s %-22s %-8s\n", "NO", "NAME", "TYPE", "LISTEN", "TARGET", "DEFAULT")
			for i, tun := range host.Tunnels {
				fmt.Printf("%-4d %-18s %-9s %-22s %-22s %-8t\n",
					i+1,
					tun.Name,
					tun.Type,
					tun.ListenAddress(),
					emptyDash(tun.TargetAddress()),
					tun.Default,
				)
			}
		}
		fmt.Println()
		fmt.Println("[a] Add  [e] Edit  [d] Delete  [s] Start  [x] Stop  [b] Back")
		switch strings.ToLower(readChoice(reader, "Action")) {
		case "b", "back":
			return nil
		case "a", "add":
			err = addTunnel(reader, configPath, cfg, index)
		case "e", "edit":
			err = editTunnel(reader, configPath, cfg, index)
		case "d", "delete":
			err = deleteTunnel(reader, configPath, cfg, index)
		case "s", "start":
			_, err = tunnel.StartDetached(ctx, tunnel.StartOptions{
				ConfigPath: configPath,
				StatePath:  statePath,
				HostName:   host.Name,
			})
		case "x", "stop":
			_, err = tunnel.Stop(ctx, statePath, host.Name, nil)
		default:
			err = fmt.Errorf("unknown action")
		}
		if err != nil {
			fmt.Printf("\nError: %v\n", err)
		} else {
			fmt.Println("\nDone.")
		}
		waitEnter(reader)
	}
}

func addTunnel(reader *bufio.Reader, configPath string, cfg model.Config, hostIndex int) error {
	tun := promptTunnel(reader, model.Tunnel{Type: model.TunnelTypeLocal, ListenHost: "127.0.0.1"})
	cfg.Hosts[hostIndex].Tunnels = append(cfg.Hosts[hostIndex].Tunnels, tun)
	return config.Save(configPath, cfg)
}

func editTunnel(reader *bufio.Reader, configPath string, cfg model.Config, hostIndex int) error {
	tunnelIndex, err := selectTunnel(reader, cfg.Hosts[hostIndex])
	if err != nil {
		return err
	}
	cfg.Hosts[hostIndex].Tunnels[tunnelIndex] = promptTunnel(reader, cfg.Hosts[hostIndex].Tunnels[tunnelIndex])
	return config.Save(configPath, cfg)
}

func deleteTunnel(reader *bufio.Reader, configPath string, cfg model.Config, hostIndex int) error {
	tunnelIndex, err := selectTunnel(reader, cfg.Hosts[hostIndex])
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
	tun.Name = promptString(reader, "Name", tun.Name)
	tun.Type = promptString(reader, "Type", tun.Type)
	tun.ListenHost = promptString(reader, "Listen host", tun.ListenHost)
	tun.ListenPort = promptInt(reader, "Listen port", tun.ListenPort)
	if tun.Type == model.TunnelTypeDynamic {
		tun.TargetHost = ""
		tun.TargetPort = 0
	} else {
		tun.TargetHost = promptString(reader, "Target host", tun.TargetHost)
		tun.TargetPort = promptInt(reader, "Target port", tun.TargetPort)
	}
	tun.Default = promptBool(reader, "Default", tun.Default)
	return tun
}

func startTunnels(ctx context.Context, reader *bufio.Reader, statePath string, configPath string) error {
	hostName := promptString(reader, "Host name", "")
	names := splitCSV(promptString(reader, "Tunnel names", ""))
	entries, err := tunnel.StartDetached(ctx, tunnel.StartOptions{
		ConfigPath:  configPath,
		StatePath:   statePath,
		HostName:    hostName,
		TunnelNames: names,
	})
	if err != nil {
		return err
	}
	for _, entry := range entries {
		fmt.Printf("Started %s/%s on %s\n", entry.HostName, entry.TunnelName, entry.ListenAddress())
	}
	return nil
}

func stopTunnels(ctx context.Context, reader *bufio.Reader, statePath string) error {
	hostName := promptString(reader, "Host name (blank for all)", "")
	names := splitCSV(promptString(reader, "Tunnel names", ""))
	entries, err := tunnel.Stop(ctx, statePath, hostName, names)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		fmt.Printf("Stopped %s/%s\n", entry.HostName, entry.TunnelName)
	}
	return nil
}

func connectHost(ctx context.Context, reader *bufio.Reader, configPath string, statePath string, cfg model.Config) error {
	index, err := selectHost(reader, cfg)
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

func selectHost(reader *bufio.Reader, cfg model.Config) (int, error) {
	if len(cfg.Hosts) == 0 {
		return 0, fmt.Errorf("no hosts configured")
	}
	value := promptString(reader, "Host number or name", "")
	if number, err := strconv.Atoi(value); err == nil {
		if number < 1 || number > len(cfg.Hosts) {
			return 0, fmt.Errorf("host number out of range")
		}
		return number - 1, nil
	}
	for i, host := range cfg.Hosts {
		if host.Name == value {
			return i, nil
		}
	}
	return 0, fmt.Errorf("host %q not found", value)
}

func selectTunnel(reader *bufio.Reader, host model.Host) (int, error) {
	if len(host.Tunnels) == 0 {
		return 0, fmt.Errorf("no tunnels configured")
	}
	value := promptString(reader, "Tunnel number or name", "")
	if number, err := strconv.Atoi(value); err == nil {
		if number < 1 || number > len(host.Tunnels) {
			return 0, fmt.Errorf("tunnel number out of range")
		}
		return number - 1, nil
	}
	for i, tun := range host.Tunnels {
		if tun.Name == value {
			return i, nil
		}
	}
	return 0, fmt.Errorf("tunnel %q not found", value)
}

func promptString(reader *bufio.Reader, label string, fallback string) string {
	if fallback == "" {
		fmt.Printf("%s: ", label)
	} else {
		fmt.Printf("%s [%s]: ", label, fallback)
	}
	value := readLine(reader)
	if value == "" {
		return fallback
	}
	return value
}

func promptInt(reader *bufio.Reader, label string, fallback int) int {
	for {
		value := promptString(reader, label, strconv.Itoa(fallback))
		number, err := strconv.Atoi(value)
		if err == nil {
			return number
		}
		fmt.Println("Please enter a number.")
	}
}

func promptBool(reader *bufio.Reader, label string, fallback bool) bool {
	defaultValue := "n"
	if fallback {
		defaultValue = "y"
	}
	for {
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

func readChoice(reader *bufio.Reader, label string) string {
	fmt.Printf("%s: ", label)
	return readLine(reader)
}

func readLine(reader *bufio.Reader) string {
	value, _ := reader.ReadString('\n')
	return strings.TrimSpace(value)
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

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
