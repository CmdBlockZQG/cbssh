//go:build linux || darwin

package platform

import (
	"errors"
	"os/exec"
	"syscall"
)

func ProcessExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func ProcessMatches(pid int, processKey string) bool {
	if pid <= 0 || processKey == "" || !ProcessExists(pid) {
		return false
	}
	current, err := ProcessKey(pid)
	return err == nil && current == processKey
}

func TerminateProcess(pid int, processKey string) error {
	if !ProcessMatches(pid, processKey) {
		return nil
	}
	return syscall.Kill(pid, syscall.SIGTERM)
}

func KillProcess(pid int, processKey string) error {
	if !ProcessMatches(pid, processKey) {
		return nil
	}
	return syscall.Kill(pid, syscall.SIGKILL)
}

func DetachCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
