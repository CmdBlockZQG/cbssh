package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"

	"github.com/CmdBlockZQG/cbssh/internal/atomicfile"
	"github.com/CmdBlockZQG/cbssh/internal/model"
	"github.com/CmdBlockZQG/cbssh/internal/platform"
)

const (
	PhaseStarting = "starting"
	PhaseActive   = "active"
)

type Registry struct {
	Version int                      `json:"version"`
	Masters map[string]MasterRuntime `json:"masters,omitempty"`
	Tunnels map[string]TunnelRuntime `json:"tunnels,omitempty"`
	History map[string]TunnelHistory `json:"history,omitempty"`
}

type MasterRuntime struct {
	ID            string    `json:"id"`
	Namespace     string    `json:"namespace"`
	Host          string    `json:"host"`
	SSHConfigPath string    `json:"ssh_config_path,omitempty"`
	ControlPath   string    `json:"control_path"`
	LogPath       string    `json:"log_path"`
	Phase         string    `json:"phase"`
	StartedAt     time.Time `json:"started_at"`
}

type TunnelRuntime struct {
	ID          string       `json:"id"`
	Namespace   string       `json:"namespace"`
	Name        string       `json:"name"`
	MasterID    string       `json:"master_id"`
	Definition  model.Tunnel `json:"definition"`
	ForwardFlag string       `json:"forward_flag"`
	ForwardSpec string       `json:"forward_spec"`
	Phase       string       `json:"phase"`
	StartedAt   time.Time    `json:"started_at"`
}

type TunnelHistory struct {
	Namespace string    `json:"namespace"`
	Name      string    `json:"name"`
	LogPath   string    `json:"log_path"`
	StartedAt time.Time `json:"started_at"`
}

type Store struct {
	Path string
}

func NewStore(path string) *Store {
	return &Store{Path: platform.ExpandPath(path)}
}

func Empty() Registry {
	return normalize(Registry{Version: 1})
}

func (s *Store) Load() (Registry, error) {
	var result Registry
	err := s.withLock(func() error {
		var err error
		result, err = s.loadUnlocked()
		return err
	})
	return result, err
}

func (s *Store) Update(update func(*Registry) error) error {
	return s.withLock(func() error {
		registry, err := s.loadUnlocked()
		if err != nil {
			return err
		}
		if err := update(&registry); err != nil {
			return err
		}
		return s.saveUnlocked(registry)
	})
}

func (s *Store) withLock(run func() error) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(s.Path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX); err != nil {
		return err
	}
	defer unix.Flock(int(lock.Fd()), unix.LOCK_UN)
	return run()
}

func (s *Store) loadUnlocked() (Registry, error) {
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return Empty(), nil
	}
	if err != nil {
		return Registry{}, err
	}
	if len(data) == 0 {
		return Empty(), nil
	}
	var registry Registry
	if err := json.Unmarshal(data, &registry); err != nil {
		return Registry{}, err
	}
	if registry.Version != 1 {
		return Registry{}, errors.New("unsupported runtime state version")
	}
	return normalize(registry), nil
}

func (s *Store) saveUnlocked(registry Registry) error {
	registry = normalize(registry)
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicfile.WriteFile(s.Path, ".runtime-*.json", data, 0o600)
}

func normalize(registry Registry) Registry {
	if registry.Version == 0 {
		registry.Version = 1
	}
	if registry.Masters == nil {
		registry.Masters = make(map[string]MasterRuntime)
	}
	if registry.Tunnels == nil {
		registry.Tunnels = make(map[string]TunnelRuntime)
	}
	if registry.History == nil {
		registry.History = make(map[string]TunnelHistory)
	}
	return registry
}
