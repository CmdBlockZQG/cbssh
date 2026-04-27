package fileui

import (
	"fmt"
	"strings"

	"github.com/cmdblock/cbssh/internal/filetransfer"
)

func formatTransferResult(verb string, result filetransfer.Result) string {
	return fmt.Sprintf("%s %s (%s)", verb, transferCount(result), formatBytes(result.Bytes))
}

func transferCount(result filetransfer.Result) string {
	parts := []string{plural(result.Files, "file")}
	if result.Directories > 0 {
		parts = append(parts, plural(result.Directories, "directory"))
	}
	return strings.Join(parts, ", ")
}

func plural(n int, singular string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %ss", n, singular)
}

func formatBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	value := float64(bytes)
	for _, unit := range []string{"KiB", "MiB", "GiB", "TiB"} {
		value /= 1024
		if value < 1024 {
			return fmt.Sprintf("%.1f %s", value, unit)
		}
	}
	return fmt.Sprintf("%.1f PiB", value/1024)
}
