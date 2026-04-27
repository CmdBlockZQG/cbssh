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
	fmt.Printf("  %scd <no|path>%s    open remote directory\n", styleBold, styleReset)
	fmt.Printf("  %sgoto <path>%s     jump to remote directory\n", styleBold, styleReset)
	fmt.Printf("  %sup [local]%s      upload local path; remote path defaults to current directory\n", styleBold, styleReset)
	fmt.Printf("  %sdown [no|path]%s  download remote path; path is relative to current directory\n", styleBold, styleReset)
	fmt.Printf("  %sh%s               toggle hidden files    %sr%s refresh    %s?%s help    %sq%s quit\n",
		styleBold, styleReset, styleBold, styleReset, styleBold, styleReset, styleBold, styleReset)
}

func (u *ui) printHelp() {
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Printf("  %scd 0%s            go to parent directory\n", styleBold, styleReset)
	fmt.Printf("  %scd <no>%s         open numbered directory\n", styleBold, styleReset)
	fmt.Printf("  %scd <path>%s       open relative, absolute, or ~/ remote directory\n", styleBold, styleReset)
	fmt.Printf("  %sgoto <path>%s     jump to remote directory\n", styleBold, styleReset)
	fmt.Printf("  %sup [local]%s      upload a local file or directory\n", styleBold, styleReset)
	fmt.Printf("  %sdown [no|path]%s  download a remote file or directory\n", styleBold, styleReset)
	fmt.Printf("  %sh%s               toggle hidden files\n", styleBold, styleReset)
	fmt.Printf("  %sr%s               refresh directory\n", styleBold, styleReset)
	fmt.Printf("  %sq%s               quit file UI\n", styleBold, styleReset)
	fmt.Println()
	fmt.Println("Remote paths entered here are resolved relative to the current remote directory unless absolute or ~/ prefixed.")
}
