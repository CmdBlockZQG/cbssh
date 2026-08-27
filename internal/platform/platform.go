package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func CanonicalPath(path string) string {
	if path == "" {
		return ""
	}
	path = ExpandPath(path)
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

func DefaultConfigPath() string {
	if value := os.Getenv("CBSSH_CONFIG"); value != "" {
		return ExpandPath(value)
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "cbssh", "tunnels.toml")
	}
	return filepath.Join(".", "tunnels.toml")
}

func DefaultStatePath() string {
	if value := os.Getenv("CBSSH_STATE"); value != "" {
		return ExpandPath(value)
	}
	if runtime.GOOS == "darwin" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "Library", "Application Support", "cbssh", "runtime.json")
		}
	}
	if value := os.Getenv("XDG_STATE_HOME"); value != "" {
		return filepath.Join(ExpandPath(value), "cbssh", "runtime.json")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "state", "cbssh", "runtime.json")
	}
	return filepath.Join(".", "runtime.json")
}

func RuntimeDir(statePath string) string {
	if statePath == "" {
		statePath = DefaultStatePath()
	}
	return filepath.Dir(ExpandPath(statePath))
}
