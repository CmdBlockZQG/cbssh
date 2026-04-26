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
