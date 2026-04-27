package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/cmdblock/cbssh/internal/atomicfile"
	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/platform"
)

func Load(path string) (model.Config, error) {
	path = configPath(path)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return Empty(), nil
	}

	var cfg model.Config
	md, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return model.Config{}, err
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, 0, len(undecoded))
		for _, key := range undecoded {
			keys = append(keys, key.String())
		}
		return model.Config{}, fmt.Errorf("unknown config fields: %s", strings.Join(keys, ", "))
	}
	cfg.Normalize()
	if err := Validate(cfg); err != nil {
		return model.Config{}, err
	}
	return cfg, nil
}

func Save(path string, cfg model.Config) error {
	path = configPath(path)
	cfg.Normalize()
	if err := Validate(cfg); err != nil {
		return err
	}

	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	enc.Indent = "  "
	if err := enc.Encode(cfg); err != nil {
		return err
	}
	return atomicfile.WriteFile(path, ".config-*.toml", buf.Bytes(), 0o600)
}

func Ensure(path string) error {
	path = configPath(path)
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return Save(path, Empty())
}

func ValidateFilePermissions(path string) []string {
	path = configPath(path)
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if info.Mode().Perm()&0o077 != 0 {
		return []string{fmt.Sprintf("config file %s is readable by other users; use chmod 600", path)}
	}
	return nil
}

func configPath(path string) string {
	path = platform.ExpandPath(path)
	if path == "" {
		return platform.DefaultConfigPath()
	}
	return path
}
