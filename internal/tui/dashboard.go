package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cmdblock/cbssh/internal/model"
)

func printDashboard(configPath string, sorted []model.Host, cfg model.Config, st model.State, sortRecent bool) {
	active := map[string]int{}
	for _, entry := range st.Tunnels {
		active[entry.HostName]++
	}
	sortLabel := "recent"
	if !sortRecent {
		sortLabel = "name"
	}
	fmt.Printf("Hosts: %d  Active Tunnels: %d  %ssort: %s%s\n", len(cfg.Hosts), len(st.Tunnels), styleDim, sortLabel, styleReset)
	fmt.Println(strings.Repeat("-", 80))
	if len(sorted) == 0 {
		fmt.Println("No hosts configured.")
	} else {
		fmt.Printf("%s%-4s %-16s %-21s %-10s %-5s %-5s%s\n", styleBold, "NO", "NAME", "HOST", "USER", "TUN", "ACT", styleReset)
		for i, host := range sorted {
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
	fmt.Println(strings.Repeat("-", 80))
	if len(st.Tunnels) > 0 {
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
		fmt.Println(strings.Repeat("-", 80))
	}
	fmt.Printf("  %s[c]%s connect  %s[f]%s files  %s[t]%s tunnels  %s[i]%s info  %s[r]%s sort  %s[v]%s validate\n",
		styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset)
	fmt.Printf("  %s[s]%s start  %s[x]%s stop  %s[a]%s add  %s[e]%s edit  %s[d]%s delete  %s[?]%s help  %s[q]%s quit\n",
		styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset)
}

func printMainHelp() {
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Printf("  %sc <host>%s               connect to host\n", styleBold, styleReset)
	fmt.Printf("  %sf <host>%s               browse files\n", styleBold, styleReset)
	fmt.Printf("  %st <host>%s               tunnel menu\n", styleBold, styleReset)
	fmt.Printf("  %si <host>%s               show host info\n", styleBold, styleReset)
	fmt.Printf("  %sr%s                      toggle sort (recent / name)\n", styleBold, styleReset)
	fmt.Printf("  %sv%s                      validate config\n", styleBold, styleReset)
	fmt.Printf("  %ss <host> [<tun>..]%s     start tunnels\n", styleBold, styleReset)
	fmt.Printf("  %sx [<host> [<tun>..]]%s   stop tunnels\n", styleBold, styleReset)
	fmt.Printf("  %sa%s                      add host\n", styleBold, styleReset)
	fmt.Printf("  %se <host>%s               edit host\n", styleBold, styleReset)
	fmt.Printf("  %sd <host>%s               delete host\n", styleBold, styleReset)
	fmt.Printf("  %s?%s                      help\n", styleBold, styleReset)
	fmt.Printf("  %sq%s                      quit\n", styleBold, styleReset)
	fmt.Println()
	fmt.Printf("  %s<host>%s = host name or number\n", styleBold, styleReset)
	fmt.Printf("  %s<tun>%s = tunnel name or number, comma/space separated\n", styleBold, styleReset)
	fmt.Println("  All commands prompt interactively when arguments are omitted.")
}
