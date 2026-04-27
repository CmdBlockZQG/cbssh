package config

import (
	"fmt"

	"github.com/cmdblock/cbssh/internal/model"
)

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
