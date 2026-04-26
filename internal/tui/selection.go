package tui

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"

	"github.com/cmdblock/cbssh/internal/model"
)

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

// resolveHostSelector accepts 1-based indexes, exact names, case-insensitive
// names, and unique prefixes in that order.
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

// resolveTunnelSelector mirrors host selection so menu commands stay compact.
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

func hostIndexByName(cfg model.Config, name string) (int, bool) {
	for i, host := range cfg.Hosts {
		if host.Name == name {
			return i, true
		}
	}
	return 0, false
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
