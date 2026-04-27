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
	"github.com/cmdblock/cbssh/internal/fileui"
	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/sshclient"
	"github.com/cmdblock/cbssh/internal/state"
	"github.com/cmdblock/cbssh/internal/tunnel"
)

var (
	runInteractiveSSH = sshclient.RunInteractive
	exitProcess       = os.Exit
)

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
	if err := runInteractiveSSH(ctx, cfg, chain); err != nil {
		return fmt.Errorf("SSH error: %w", err)
	}
	exitProcess(0)
	return nil
}

func browseFiles(ctx context.Context, reader *bufio.Reader, statePath string, cfg model.Config, selector string) error {
	index, err := selectHost(reader, cfg, selector)
	if err != nil {
		return err
	}
	host := cfg.Hosts[index]
	_ = state.MarkHostUsed(statePath, host.Name, time.Now())
	if err := fileui.Run(ctx, cfg, host.Name); err != nil {
		addActionError("File error: %v", err)
	}
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
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("Host: %s\n", host.Address())
	fmt.Printf("User: %s\n", host.User)
	jump := strings.Join(chain, " -> ")
	fmt.Printf("Jump: %s\n", emptyDash(jump))
	authLine := host.Auth.Type
	if host.Auth.Type == model.AuthTypeKey {
		authLine += " " + host.Auth.KeyPath
	}
	fmt.Printf("Auth: %s\n", authLine)
	fmt.Println(strings.Repeat("-", 80))
	if len(host.Tunnels) == 0 {
		fmt.Println("No tunnels configured.")
	} else {
		fmt.Printf("%s%-4s %-16s %-1s %-21s %-21s %-3s %-7s%s\n",
			styleBold, "NO", "NAME", "T", "LISTEN", "TARGET", "DEF", "PID", styleReset)
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
			fmt.Printf(" %-3d %-16s %-1s %-21s %-21s %-3d %-7s\n",
				i+1, tun.Name, model.TunnelTypeCode(tun.Type),
				tun.ListenAddress(), emptyDash(tun.TargetAddress()),
				def, pid)
		}
	}
	fmt.Println(strings.Repeat("-", 80))
	waitEnter(reader)
	return nil
}
