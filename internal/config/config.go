package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/CmdBlockZQG/cbssh/internal/atomicfile"
	"github.com/CmdBlockZQG/cbssh/internal/model"
	"github.com/CmdBlockZQG/cbssh/internal/platform"
)

var tunnelNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

var ErrAlreadyExists = errors.New("tunnel config already exists")

func Load(path string) (model.Config, error) {
	path = platform.ExpandPath(path)
	var cfg model.Config
	metadata, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return model.Config{}, fmt.Errorf("load tunnel config %s: %w", path, err)
	}
	if undecoded := metadata.Undecoded(); len(undecoded) > 0 {
		fields := make([]string, 0, len(undecoded))
		for _, key := range undecoded {
			fields = append(fields, key.String())
		}
		sort.Strings(fields)
		return model.Config{}, fmt.Errorf("unknown config fields: %s", strings.Join(fields, ", "))
	}
	for i := range cfg.Tunnels {
		cfg.Tunnels[i].Normalize()
	}
	if err := Validate(cfg); err != nil {
		return model.Config{}, err
	}
	return cfg, nil
}

func Validate(cfg model.Config) error {
	if cfg.Version != 1 {
		return fmt.Errorf("unsupported config version %d; want 1", cfg.Version)
	}
	seen := make(map[string]struct{}, len(cfg.Tunnels))
	for i, tun := range cfg.Tunnels {
		prefix := fmt.Sprintf("tunnels[%d]", i)
		if tun.Name == "" || !tunnelNamePattern.MatchString(tun.Name) {
			return fmt.Errorf("%s.name must match %s", prefix, tunnelNamePattern.String())
		}
		if _, exists := seen[tun.Name]; exists {
			return fmt.Errorf("duplicate tunnel name %q", tun.Name)
		}
		seen[tun.Name] = struct{}{}
		if tun.Host == "" || strings.HasPrefix(tun.Host, "-") {
			return fmt.Errorf("%s.host must be a non-option OpenSSH destination", prefix)
		}
		if tun.BindPort < 1 || tun.BindPort > 65535 {
			return fmt.Errorf("%s.bind_port must be between 1 and 65535", prefix)
		}
		switch tun.Type {
		case model.TunnelTypeLocal, model.TunnelTypeRemote:
			if tun.TargetHost == "" || tun.TargetPort < 1 || tun.TargetPort > 65535 {
				return fmt.Errorf("%s requires target_host and target_port between 1 and 65535", prefix)
			}
		case model.TunnelTypeDynamic:
			if tun.TargetHost != "" || tun.TargetPort != 0 {
				return fmt.Errorf("%s dynamic tunnel must not define a target", prefix)
			}
		default:
			return fmt.Errorf("%s.type must be local, remote, or dynamic", prefix)
		}
	}
	return nil
}

func Init(path string) error {
	path = platform.ExpandPath(path)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%w: %s", ErrAlreadyExists, path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data := []byte("version = 1\n\n# Add [[tunnels]] entries here.\n")
	return atomicfile.WriteFile(path, ".tunnels-*.toml", data, 0o600)
}

func Find(cfg model.Config, name string) (model.Tunnel, bool) {
	for _, tun := range cfg.Tunnels {
		if tun.Name == name {
			return tun, true
		}
	}
	return model.Tunnel{}, false
}

func Select(cfg model.Config, names []string, all bool) ([]model.Tunnel, error) {
	if all {
		if len(names) != 0 {
			return nil, errors.New("tunnel names and --all cannot be used together")
		}
		return append([]model.Tunnel(nil), cfg.Tunnels...), nil
	}
	if len(names) == 0 {
		return nil, errors.New("provide at least one tunnel name or use --all")
	}
	selected := make([]model.Tunnel, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("duplicate tunnel name %q", name)
		}
		seen[name] = struct{}{}
		tun, ok := Find(cfg, name)
		if !ok {
			return nil, fmt.Errorf("tunnel %q is not configured", name)
		}
		selected = append(selected, tun)
	}
	return selected, nil
}
