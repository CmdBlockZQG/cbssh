//go:build linux

package platform

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

func ProcessKey(pid int) (string, error) {
	startTicks, err := linuxProcessStartTicks(pid)
	if err != nil {
		return "", err
	}
	bootID, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("linux:%s:%s", strings.TrimSpace(string(bootID)), startTicks), nil
}

func linuxProcessStartTicks(pid int) (string, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "", err
	}
	line := string(data)
	commEnd := strings.LastIndex(line, ") ")
	if commEnd == -1 {
		return "", errors.New("invalid /proc stat format")
	}
	fields := strings.Fields(line[commEnd+2:])
	if len(fields) < 20 {
		return "", errors.New("missing process start time in /proc stat")
	}
	return fields[19], nil
}
