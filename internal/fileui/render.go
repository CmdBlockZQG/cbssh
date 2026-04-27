package fileui

import (
	"fmt"
	"strings"
)

func (u *ui) render() {
	hiddenLabel := "hidden files: off"
	if u.showDot {
		hiddenLabel = "hidden files: on"
	}
	fmt.Printf("Host: %s%s%s  Remote: %s%s%s  %s%s%s\n", styleBold, u.hostName, styleReset, styleBold, u.cwd, styleReset, styleDim, hiddenLabel, styleReset)
	fmt.Println(strings.Repeat("-", 80))
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
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("  %s[c]%s cd  %s[u]%s upload  %s[d]%s download  %s[h]%s hidden  %s[r]%s refresh\n",
		styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset)
	fmt.Printf("  %s[?]%s help  %s[q]%s quit\n",
		styleBold, styleReset, styleBold, styleReset)
}

func (u *ui) printMessage() {
	if u.message != "" {
		fmt.Println()
		fmt.Println("  " + u.message)
		u.message = ""
		u.waitEnter()
	}
}

func (u *ui) printHelp() {
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Printf("  %sc <no>%s               cd numbered directory\n", styleBold, styleReset)
	fmt.Printf("  %sc <path>%s             cd path\n", styleBold, styleReset)
	fmt.Printf("  %su [local [remote]]%s   upload a local file or directory\n", styleBold, styleReset)
	fmt.Printf("  %sd [remote [local]]%s   download a remote file or directory\n", styleBold, styleReset)
	fmt.Printf("  %sh%s                    toggle hidden files\n", styleBold, styleReset)
	fmt.Printf("  %sr%s                    refresh\n", styleBold, styleReset)
	fmt.Printf("  %s?%s                    help\n", styleBold, styleReset)
	fmt.Printf("  %sq%s                    quit\n", styleBold, styleReset)
	fmt.Println()
	fmt.Println("  All commands prompt interactively when arguments are omitted.")
}
