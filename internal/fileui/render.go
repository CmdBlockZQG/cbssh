package fileui

import (
	"fmt"
	"strings"
)

func (u *ui) render() {
	fmt.Printf("%s%s cbssh files%s\n", styleCyan, styleBold, styleReset)
	fmt.Println(strings.Repeat("-", 80))
	hiddenLabel := "hidden files: off"
	if u.showDot {
		hiddenLabel = "hidden files: on"
	}
	fmt.Printf("Host: %s%s%s  Remote: %s%s%s  %s%s%s\n\n", styleBold, u.hostName, styleReset, styleBold, u.cwd, styleReset, styleDim, hiddenLabel, styleReset)
	if u.message != "" {
		fmt.Println(u.message)
		u.message = ""
		fmt.Println()
	}
	fmt.Printf("%s%-4s %-5s %-10s %s%s\n", styleBold, "NO", "TYPE", "SIZE", "NAME", styleReset)
	fmt.Printf(" %-3d %-5s %-10s %s\n", 0, "dir", "-", "..")
	for i, entry := range u.visible {
		kind := "file"
		name := entry.Name
		if entry.IsDir {
			kind = "dir"
			name += "/"
		}
		fmt.Printf(" %-3d %-5s %-10s %s\n", i+1, kind, formatBytes(entry.Size), name)
	}
	fmt.Println()
	fmt.Printf("  %s[c]%s cd  %s[u]%s upload  %s[d]%s download  %s[h]%s hidden  %s[r]%s refresh\n",
		styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset)
	fmt.Printf("  %s[?]%s help  %s[q]%s quit\n",
		styleBold, styleReset, styleBold, styleReset)
}

func (u *ui) printHelp() {
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Printf("  %sc 0%s             go to parent directory\n", styleBold, styleReset)
	fmt.Printf("  %sc <no>%s          open numbered directory\n", styleBold, styleReset)
	fmt.Printf("  %sc <path>%s        open relative, absolute, or ~/ remote directory\n", styleBold, styleReset)
	fmt.Printf("  %scd <path>%s       alias for c\n", styleBold, styleReset)
	fmt.Printf("  %su [local] [remote]%s upload a local file or directory\n", styleBold, styleReset)
	fmt.Printf("  %sd [remote] [local]%s download a remote file or directory\n", styleBold, styleReset)
	fmt.Printf("  %sh%s               toggle hidden files\n", styleBold, styleReset)
	fmt.Printf("  %sr%s               refresh directory\n", styleBold, styleReset)
	fmt.Printf("  %sq%s               quit file UI\n", styleBold, styleReset)
	fmt.Println()
	fmt.Println("u and d prompt only for arguments that were not provided.")
}
