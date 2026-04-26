package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/cmdblock/cbssh/internal/model"
)

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
		fmt.Println(strings.Repeat("-", 80))
	} else {
		fmt.Println(strings.Repeat("-", 80))
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
