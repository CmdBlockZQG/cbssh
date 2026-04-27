package config

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/cmdblock/cbssh/internal/model"
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func Validate(cfg model.Config) error {
	cfg.Normalize()
	if err := validateHostKeyCheck(cfg.HostKeyCheck); err != nil {
		return err
	}
	seenHosts := map[string]struct{}{}
	for _, host := range cfg.Hosts {
		if err := validateHost(host); err != nil {
			return err
		}
		if _, exists := seenHosts[host.Name]; exists {
			return fmt.Errorf("duplicate host name %q", host.Name)
		}
		seenHosts[host.Name] = struct{}{}

		seenTunnels := map[string]struct{}{}
		for _, tun := range host.Tunnels {
			if err := validateTunnel(host.Name, tun); err != nil {
				return err
			}
			if _, exists := seenTunnels[tun.Name]; exists {
				return fmt.Errorf("duplicate tunnel name %q on host %q", tun.Name, host.Name)
			}
			seenTunnels[tun.Name] = struct{}{}
		}
	}

	for _, host := range cfg.Hosts {
		if host.Jump != "" {
			if _, ok := seenHosts[host.Jump]; !ok {
				return fmt.Errorf("host %q references missing jump host %q", host.Name, host.Jump)
			}
		}
		if _, err := ResolveChain(cfg, host.Name); err != nil {
			return err
		}
	}

	return nil
}

func validateHostKeyCheck(value string) error {
	switch value {
	case "", "insecure", "known_hosts", "known-hosts":
		return nil
	default:
		return fmt.Errorf("unsupported host_key_check %q", value)
	}
}

func validateHost(host model.Host) error {
	if host.Name == "" {
		return errors.New("host name is required")
	}
	if !namePattern.MatchString(host.Name) {
		return fmt.Errorf("invalid host name %q", host.Name)
	}
	if host.Host == "" {
		return fmt.Errorf("host %q address is required", host.Name)
	}
	if host.User == "" {
		return fmt.Errorf("host %q user is required", host.Name)
	}
	if !validPort(host.Port) {
		return fmt.Errorf("host %q port must be between 1 and 65535", host.Name)
	}
	switch host.Auth.Type {
	case model.AuthTypeKey:
		if host.Auth.KeyPath == "" {
			return fmt.Errorf("host %q key_path is required", host.Name)
		}
	case model.AuthTypePassword:
		if host.Auth.Password == "" {
			return fmt.Errorf("host %q password is required", host.Name)
		}
	default:
		return fmt.Errorf("host %q has unsupported auth type %q", host.Name, host.Auth.Type)
	}
	return nil
}

func validateTunnel(hostName string, tun model.Tunnel) error {
	if tun.Name == "" {
		return fmt.Errorf("host %q has a tunnel without name", hostName)
	}
	if !namePattern.MatchString(tun.Name) {
		return fmt.Errorf("host %q has invalid tunnel name %q", hostName, tun.Name)
	}
	if tun.ListenHost == "" {
		return fmt.Errorf("tunnel %q on host %q listen_host is required", tun.Name, hostName)
	}
	if !validPort(tun.ListenPort) {
		return fmt.Errorf("tunnel %q on host %q listen_port must be between 1 and 65535", tun.Name, hostName)
	}
	switch tun.Type {
	case model.TunnelTypeLocal, model.TunnelTypeRemote:
		if tun.TargetHost == "" {
			return fmt.Errorf("tunnel %q on host %q target_host is required", tun.Name, hostName)
		}
		if !validPort(tun.TargetPort) {
			return fmt.Errorf("tunnel %q on host %q target_port must be between 1 and 65535", tun.Name, hostName)
		}
	case model.TunnelTypeDynamic:
	default:
		return fmt.Errorf("tunnel %q on host %q has unsupported type %q", tun.Name, hostName, tun.Type)
	}
	return nil
}

func validPort(port int) bool {
	return port >= 1 && port <= 65535
}
