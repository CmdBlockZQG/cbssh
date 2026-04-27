package config

import (
	"fmt"

	"github.com/cmdblock/cbssh/internal/model"
)

// SelectTunnels resolves requested names, or the host defaults when no names are provided.
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
	seen := map[string]struct{}{}
	for _, name := range names {
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("duplicate selected tunnel %q on host %q", name, host.Name)
		}
		seen[name] = struct{}{}
		tun, ok := tunnelMap[name]
		if !ok {
			return nil, fmt.Errorf("host %q has no tunnel named %q", host.Name, name)
		}
		selected = append(selected, tun)
	}
	return selected, nil
}
