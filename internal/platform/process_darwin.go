//go:build darwin

package platform

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func ProcessKey(pid int) (string, error) {
	info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return "", err
	}
	if info == nil || int(info.Proc.P_pid) != pid {
		return "", fmt.Errorf("process %d not found", pid)
	}
	start := info.Proc.P_starttime
	return fmt.Sprintf("darwin:%d:%d", start.Sec, start.Usec), nil
}
