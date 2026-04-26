package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/platform"
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func Empty() model.Config {
	cfg := model.Config{}
	cfg.Normalize()
	return cfg
}

func Load(path string) (model.Config, error) {
	path = platform.ExpandPath(path)
	if path == "" {
		path = platform.DefaultConfigPath()
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return Empty(), nil
	}

	var cfg model.Config
	md, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return model.Config{}, err
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, 0, len(undecoded))
		for _, key := range undecoded {
			keys = append(keys, key.String())
		}
		return model.Config{}, fmt.Errorf("unknown config fields: %s", strings.Join(keys, ", "))
	}
	cfg.Normalize()
	if err := Validate(cfg); err != nil {
		return model.Config{}, err
	}
	return cfg, nil
}

func Save(path string, cfg model.Config) error {
	path = platform.ExpandPath(path)
	if path == "" {
		path = platform.DefaultConfigPath()
	}
	cfg.Normalize()
	if err := Validate(cfg); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.toml")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	enc := toml.NewEncoder(tmp)
	enc.Indent = "  "
	if err := enc.Encode(cfg); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func Ensure(path string) error {
	path = platform.ExpandPath(path)
	if path == "" {
		path = platform.DefaultConfigPath()
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return Save(path, Empty())
}

func Validate(cfg model.Config) error {
	cfg.Normalize()
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

func ValidateFilePermissions(path string) []string {
	path = platform.ExpandPath(path)
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if info.Mode().Perm()&0o077 != 0 {
		return []string{fmt.Sprintf("config file %s is readable by other users; use chmod 600", path)}
	}
	return nil
}

func ResolveHost(cfg model.Config, name string) (model.Host, bool) {
	for _, host := range cfg.Hosts {
		if host.Name == name {
			return host, true
		}
	}
	return model.Host{}, false
}

func ResolveChain(cfg model.Config, name string) ([]model.Host, error) {
	hostMap := make(map[string]model.Host, len(cfg.Hosts))
	for _, host := range cfg.Hosts {
		hostMap[host.Name] = host
	}
	visited := map[string]bool{}
	var reversed []model.Host
	current := name
	for current != "" {
		if visited[current] {
			return nil, fmt.Errorf("jump cycle detected at host %q", current)
		}
		visited[current] = true
		host, ok := hostMap[current]
		if !ok {
			return nil, fmt.Errorf("host %q not found", current)
		}
		reversed = append(reversed, host)
		current = host.Jump
	}
	chain := make([]model.Host, len(reversed))
	for i := range reversed {
		chain[i] = reversed[len(reversed)-1-i]
	}
	return chain, nil
}

func ResolveJumpNames(cfg model.Config, name string) ([]string, error) {
	chain, err := ResolveChain(cfg, name)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(chain))
	for _, host := range chain {
		names = append(names, host.Name)
	}
	return names, nil
}

func SelectTunnels(host model.Host, names []string) ([]model.Tunnel, error) {
	if len(names) == 0 {
		var selected []model.Tunnel
		for _, tun := range host.Tunnels {
			if tun.Default {
				selected = append(selected, tun)
			}
		}
		if len(selected) == 0 {
			return nil, fmt.Errorf("host %q has no default tunnels", host.Name)
		}
		return selected, nil
	}

	tunnelMap := make(map[string]model.Tunnel, len(host.Tunnels))
	for _, tun := range host.Tunnels {
		tunnelMap[tun.Name] = tun
	}
	selected := make([]model.Tunnel, 0, len(names))
	for _, name := range names {
		tun, ok := tunnelMap[name]
		if !ok {
			return nil, fmt.Errorf("host %q has no tunnel named %q", host.Name, name)
		}
		selected = append(selected, tun)
	}
	return selected, nil
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
