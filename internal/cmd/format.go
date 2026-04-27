package cmd

import "time"

const cliBold = "\033[1m"
const cliReset = "\033[0m"

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Format("2006-01-02 15:04:05")
}
