package platform

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

func ExpandPath(path string) string {
	if path == "" {
		return path
	}
	if strings.HasPrefix(path, "~/") || path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			if path == "~" {
				return home
			}
			return filepath.Join(home, path[2:])
		}
	}
	return os.ExpandEnv(path)
}

func DefaultConfigPath() string {
	if value := os.Getenv("CBSSH_CONFIG"); value != "" {
		return ExpandPath(value)
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "cbssh", "config.toml")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "cbssh", "config.toml")
	}
	return filepath.Join(".", "config.toml")
}

func DefaultStatePath() string {
	if value := os.Getenv("CBSSH_STATE"); value != "" {
		return ExpandPath(value)
	}
	if runtime.GOOS == "darwin" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "Library", "Application Support", "cbssh", "state.json")
		}
	}
	if value := os.Getenv("XDG_STATE_HOME"); value != "" {
		return filepath.Join(ExpandPath(value), "cbssh", "state.json")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "state", "cbssh", "state.json")
	}
	return filepath.Join(".", "state.json")
}

func DefaultLogDir() string {
	if value := os.Getenv("CBSSH_LOG_DIR"); value != "" {
		return ExpandPath(value)
	}
	return filepath.Join(filepath.Dir(DefaultStatePath()), "logs")
}

func ProcessExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func TerminateProcess(pid int) error {
	if pid <= 0 {
		return nil
	}
	if !ProcessExists(pid) {
		return nil
	}
	return syscall.Kill(pid, syscall.SIGTERM)
}

func KillProcess(pid int) error {
	if pid <= 0 {
		return nil
	}
	if !ProcessExists(pid) {
		return nil
	}
	return syscall.Kill(pid, syscall.SIGKILL)
}

func DetachCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
