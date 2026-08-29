package platform

import (
	"os"
	"path/filepath"
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

type Layout struct {
	Dir        string
	ConfigPath string
	StatePath  string
}

func DefaultDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".cbssh")
	}
	return filepath.Join(".", ".cbssh")
}

func ResolveLayout(dir string) Layout {
	if dir == "" {
		dir = DefaultDir()
	}
	dir = filepath.Clean(ExpandPath(dir))
	return Layout{
		Dir:        dir,
		ConfigPath: filepath.Join(dir, "tunnels.toml"),
		StatePath:  filepath.Join(dir, "runtime.json"),
	}
}
