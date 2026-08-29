package tunnel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"golang.org/x/sys/unix"

	"github.com/CmdBlockZQG/cbssh/internal/model"
	"github.com/CmdBlockZQG/cbssh/internal/openssh"
	"github.com/CmdBlockZQG/cbssh/internal/platform"
	"github.com/CmdBlockZQG/cbssh/internal/state"
)

type Options struct {
	Dir           string
	SSHConfigPath string
	Runner        openssh.Runner
}

type Manager struct {
	Namespace     string
	SSHConfigPath string
	Dir           string
	Store         *state.Store
	Runner        openssh.Runner
}

type Action struct {
	Name     string
	Host     string
	Changed  bool
	MasterID string
}

type Status struct {
	Name       string
	Definition model.Tunnel
	State      string
	MasterID   string
	StartedAt  time.Time
	Orphaned   bool
}

func NewManager(opts Options) *Manager {
	layout := platform.ResolveLayout(opts.Dir)
	runner := opts.Runner
	if runner == nil {
		runner = openssh.NewCommandRunner()
	}
	return &Manager{
		Namespace:     platform.CanonicalPath(layout.ConfigPath),
		SSHConfigPath: platform.CanonicalPath(opts.SSHConfigPath),
		Dir:           layout.Dir,
		Store:         state.NewStore(layout.StatePath),
		Runner:        runner,
	}
}

func (m *Manager) Start(ctx context.Context, definition model.Tunnel) (Action, error) {
	desired := m.masterFor(definition)
	result := Action{Name: definition.Name, Host: definition.Host, MasterID: desired.ID}
	err := m.withMasterLock(desired.ID, func() error {
		if err := m.reconcileMasterLocked(ctx, desired.ID); err != nil {
			return err
		}
		registry, err := m.Store.Load()
		if err != nil {
			return err
		}
		tunnelID := m.tunnelID(definition.Name)
		if active, exists := registry.Tunnels[tunnelID]; exists {
			if active.Phase == state.PhaseActive && active.MasterID == desired.ID && active.Definition == definition {
				return nil
			}
			return fmt.Errorf("tunnel %q is running with a different definition; use restart", definition.Name)
		}

		flag, spec, err := openssh.ForwardArguments(definition)
		if err != nil {
			return err
		}
		master, masterExists := registry.Masters[desired.ID]
		newMaster := !masterExists
		now := time.Now()
		if newMaster {
			master = desired
			master.LogPath = filepath.Join(m.Dir, "logs", fmt.Sprintf("%s-%d.log", master.ID, now.UnixNano()))
			if err := m.prepareMaster(master); err != nil {
				return err
			}
		}
		pending := state.TunnelRuntime{
			ID: tunnelID, Namespace: m.Namespace, Name: definition.Name,
			MasterID: desired.ID, Definition: definition,
			ForwardFlag: flag, ForwardSpec: spec, Phase: state.PhaseStarting, StartedAt: now,
		}
		if err := m.Store.Update(func(registry *state.Registry) error {
			if _, exists := registry.Tunnels[tunnelID]; exists {
				return fmt.Errorf("tunnel %q changed concurrently", definition.Name)
			}
			if newMaster {
				master.Phase = state.PhaseStarting
				master.StartedAt = now
				registry.Masters[master.ID] = master
			}
			registry.Tunnels[tunnelID] = pending
			registry.History[tunnelID] = state.TunnelHistory{Namespace: m.Namespace, Name: definition.Name, LogPath: master.LogPath, StartedAt: now}
			return nil
		}); err != nil {
			return err
		}

		if newMaster {
			if err := m.Runner.StartMaster(ctx, opensshMaster(master)); err != nil {
				cleanupErr := m.cleanupStartedMaster(master)
				stateErr := m.removePending(master.ID, tunnelID, true)
				return errors.Join(err, cleanupErr, stateErr)
			}
		}
		if err := m.Runner.Forward(ctx, opensshMaster(master), flag, spec); err != nil {
			if newMaster {
				_ = m.Runner.Exit(context.Background(), opensshMaster(master))
			}
			_ = m.removePending(master.ID, tunnelID, newMaster)
			return fmt.Errorf("%w (log: %s)", err, master.LogPath)
		}

		if err := m.Store.Update(func(registry *state.Registry) error {
			current, exists := registry.Tunnels[tunnelID]
			if !exists || current.Phase != state.PhaseStarting {
				return fmt.Errorf("pending state for tunnel %q disappeared", definition.Name)
			}
			master.Phase = state.PhaseActive
			registry.Masters[master.ID] = master
			current.Phase = state.PhaseActive
			registry.Tunnels[tunnelID] = current
			return nil
		}); err != nil {
			cancelErr := m.Runner.Cancel(context.Background(), opensshMaster(master), flag, spec)
			if newMaster || cancelErr != nil {
				_ = m.Runner.Exit(context.Background(), opensshMaster(master))
				_ = m.removeMaster(master.ID)
			} else {
				_ = m.removePending(master.ID, tunnelID, false)
			}
			return err
		}
		result.Changed = true
		return nil
	})
	return result, err
}

func (m *Manager) Stop(ctx context.Context, name string) (Action, error) {
	result := Action{Name: name}
	registry, err := m.Store.Load()
	if err != nil {
		return result, err
	}
	tunnelID := m.tunnelID(name)
	runtimeTunnel, exists := registry.Tunnels[tunnelID]
	if !exists {
		return result, nil
	}
	result.Host = runtimeTunnel.Definition.Host
	result.MasterID = runtimeTunnel.MasterID
	err = m.withMasterLock(runtimeTunnel.MasterID, func() error {
		if err := m.reconcileMasterLocked(ctx, runtimeTunnel.MasterID); err != nil {
			return err
		}
		registry, err := m.Store.Load()
		if err != nil {
			return err
		}
		runtimeTunnel, exists := registry.Tunnels[tunnelID]
		if !exists {
			return nil
		}
		master, exists := registry.Masters[runtimeTunnel.MasterID]
		if !exists {
			return m.Store.Update(func(registry *state.Registry) error {
				delete(registry.Tunnels, tunnelID)
				return nil
			})
		}
		activeCount := 0
		for _, candidate := range registry.Tunnels {
			if candidate.MasterID == master.ID && candidate.Phase == state.PhaseActive {
				activeCount++
			}
		}
		if activeCount <= 1 {
			if err := m.Runner.Exit(ctx, opensshMaster(master)); err != nil {
				alive, checkErr := m.Runner.CheckMaster(ctx, opensshMaster(master))
				if checkErr != nil {
					return errors.Join(err, checkErr)
				}
				if alive {
					return err
				}
			}
			if err := m.removeMaster(master.ID); err != nil {
				return err
			}
			_ = os.Remove(master.ControlPath)
			result.Changed = true
			return nil
		}

		if err := m.Runner.Cancel(ctx, opensshMaster(master), runtimeTunnel.ForwardFlag, runtimeTunnel.ForwardSpec); err != nil {
			alive, checkErr := m.Runner.CheckMaster(ctx, opensshMaster(master))
			if checkErr != nil {
				return errors.Join(err, checkErr)
			}
			if alive {
				return err
			}
			if cleanupErr := m.removeMaster(master.ID); cleanupErr != nil {
				return cleanupErr
			}
			result.Changed = true
			return nil
		}
		if err := m.Store.Update(func(registry *state.Registry) error {
			delete(registry.Tunnels, tunnelID)
			return nil
		}); err != nil {
			rollbackErr := m.Runner.Forward(context.Background(), opensshMaster(master), runtimeTunnel.ForwardFlag, runtimeTunnel.ForwardSpec)
			if rollbackErr != nil {
				_ = m.Runner.Exit(context.Background(), opensshMaster(master))
				_ = m.removeMaster(master.ID)
			}
			return errors.Join(err, rollbackErr)
		}
		result.Changed = true
		return nil
	})
	return result, err
}

func (m *Manager) Reconcile(ctx context.Context) error {
	registry, err := m.Store.Load()
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(registry.Masters))
	for id, master := range registry.Masters {
		if master.Namespace == m.Namespace {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := m.withMasterLock(id, func() error { return m.reconcileMasterLocked(ctx, id) }); err != nil {
			return err
		}
	}
	return m.removeOrphanedTunnels()
}

func (m *Manager) Status(ctx context.Context, configured []model.Tunnel, names []string) ([]Status, error) {
	if err := m.Reconcile(ctx); err != nil {
		return nil, err
	}
	registry, err := m.Store.Load()
	if err != nil {
		return nil, err
	}
	definitions := make(map[string]model.Tunnel, len(configured))
	selected := make(map[string]struct{})
	for _, definition := range configured {
		definitions[definition.Name] = definition
		if len(names) == 0 {
			selected[definition.Name] = struct{}{}
		}
	}
	if len(names) > 0 {
		for _, name := range names {
			selected[name] = struct{}{}
		}
	}
	for _, active := range registry.Tunnels {
		if active.Namespace == m.Namespace && len(names) == 0 {
			selected[active.Name] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(selected))
	for name := range selected {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	rows := make([]Status, 0, len(ordered))
	for _, name := range ordered {
		definition, configuredOK := definitions[name]
		active, activeOK := registry.Tunnels[m.tunnelID(name)]
		if !configuredOK && !activeOK {
			return nil, fmt.Errorf("tunnel %q is neither configured nor active", name)
		}
		row := Status{Name: name, Definition: definition, State: "stopped"}
		if activeOK {
			row.Definition = active.Definition
			row.State = "running"
			row.MasterID = active.MasterID
			row.StartedAt = active.StartedAt
			row.Orphaned = !configuredOK
			if configuredOK && (active.Definition != definition || active.MasterID != m.masterFor(definition).ID) {
				row.State = "changed"
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (m *Manager) ActiveNames(ctx context.Context) ([]string, error) {
	if err := m.Reconcile(ctx); err != nil {
		return nil, err
	}
	registry, err := m.Store.Load()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, active := range registry.Tunnels {
		if active.Namespace == m.Namespace && active.Phase == state.PhaseActive {
			names = append(names, active.Name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func (m *Manager) LogPath(name string) (string, error) {
	registry, err := m.Store.Load()
	if err != nil {
		return "", err
	}
	tunnelID := m.tunnelID(name)
	if active, ok := registry.Tunnels[tunnelID]; ok {
		if master, ok := registry.Masters[active.MasterID]; ok {
			return master.LogPath, nil
		}
	}
	if history, ok := registry.History[tunnelID]; ok {
		return history.LogPath, nil
	}
	return "", fmt.Errorf("no log is available for tunnel %q", name)
}

func (m *Manager) reconcileMasterLocked(ctx context.Context, masterID string) error {
	registry, err := m.Store.Load()
	if err != nil {
		return err
	}
	master, exists := registry.Masters[masterID]
	if !exists {
		return nil
	}
	alive, err := m.Runner.CheckMaster(ctx, opensshMaster(master))
	if err != nil {
		return err
	}
	if master.Phase == state.PhaseStarting {
		if alive {
			if err := m.Runner.Exit(ctx, opensshMaster(master)); err != nil {
				return err
			}
		}
		_ = os.Remove(master.ControlPath)
		return m.removeMaster(master.ID)
	}
	if !alive {
		_ = os.Remove(master.ControlPath)
		return m.removeMaster(master.ID)
	}

	var pending []state.TunnelRuntime
	activeCount := 0
	for _, runtimeTunnel := range registry.Tunnels {
		if runtimeTunnel.MasterID != master.ID {
			continue
		}
		if runtimeTunnel.Phase == state.PhaseStarting {
			pending = append(pending, runtimeTunnel)
		} else if runtimeTunnel.Phase == state.PhaseActive {
			activeCount++
		}
	}
	for _, runtimeTunnel := range pending {
		if err := m.Runner.Cancel(ctx, opensshMaster(master), runtimeTunnel.ForwardFlag, runtimeTunnel.ForwardSpec); err != nil {
			stillAlive, checkErr := m.Runner.CheckMaster(ctx, opensshMaster(master))
			if checkErr != nil {
				return errors.Join(err, checkErr)
			}
			if stillAlive {
				if exitErr := m.Runner.Exit(ctx, opensshMaster(master)); exitErr != nil {
					return errors.Join(err, exitErr)
				}
			}
			_ = os.Remove(master.ControlPath)
			return m.removeMaster(master.ID)
		}
	}
	if len(pending) > 0 {
		if err := m.Store.Update(func(registry *state.Registry) error {
			for _, runtimeTunnel := range pending {
				delete(registry.Tunnels, runtimeTunnel.ID)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	if activeCount == 0 {
		if err := m.Runner.Exit(ctx, opensshMaster(master)); err != nil {
			return err
		}
		_ = os.Remove(master.ControlPath)
		return m.removeMaster(master.ID)
	}
	return nil
}

func (m *Manager) prepareMaster(master state.MasterRuntime) error {
	if err := os.MkdirAll(filepath.Dir(master.ControlPath), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(master.LogPath), 0o700); err != nil {
		return err
	}
	if err := m.cleanupStartedMaster(master); err != nil {
		return err
	}
	logFile, err := os.OpenFile(master.LogPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	return logFile.Close()
}

func (m *Manager) cleanupStartedMaster(master state.MasterRuntime) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	alive, err := m.Runner.CheckMaster(ctx, opensshMaster(master))
	if err != nil {
		return fmt.Errorf("check partially started Master: %w", err)
	}
	if alive {
		if err := m.Runner.Exit(ctx, opensshMaster(master)); err != nil {
			return fmt.Errorf("stop partially started Master: %w", err)
		}
	}
	_ = os.Remove(master.ControlPath)
	return nil
}

func (m *Manager) removePending(masterID, tunnelID string, removeMaster bool) error {
	return m.Store.Update(func(registry *state.Registry) error {
		delete(registry.Tunnels, tunnelID)
		if removeMaster {
			delete(registry.Masters, masterID)
		}
		return nil
	})
}

func (m *Manager) removeMaster(masterID string) error {
	return m.Store.Update(func(registry *state.Registry) error {
		delete(registry.Masters, masterID)
		for id, runtimeTunnel := range registry.Tunnels {
			if runtimeTunnel.MasterID == masterID {
				delete(registry.Tunnels, id)
			}
		}
		return nil
	})
}

func (m *Manager) removeOrphanedTunnels() error {
	registry, err := m.Store.Load()
	if err != nil {
		return err
	}
	var orphaned []string
	for id, runtimeTunnel := range registry.Tunnels {
		if runtimeTunnel.Namespace != m.Namespace {
			continue
		}
		if _, exists := registry.Masters[runtimeTunnel.MasterID]; !exists {
			orphaned = append(orphaned, id)
		}
	}
	if len(orphaned) == 0 {
		return nil
	}
	return m.Store.Update(func(registry *state.Registry) error {
		for _, id := range orphaned {
			if runtimeTunnel, exists := registry.Tunnels[id]; exists {
				if _, masterExists := registry.Masters[runtimeTunnel.MasterID]; !masterExists {
					delete(registry.Tunnels, id)
				}
			}
		}
		return nil
	})
}

func (m *Manager) masterFor(definition model.Tunnel) state.MasterRuntime {
	id := stableID(m.Namespace, m.SSHConfigPath, definition.Host)
	return state.MasterRuntime{
		ID: id, Namespace: m.Namespace, Host: definition.Host, SSHConfigPath: m.SSHConfigPath,
		ControlPath: filepath.Join(m.Dir, "sockets", id+".sock"),
		LogPath:     filepath.Join(m.Dir, "logs", id+".log"),
		Phase:       state.PhaseActive,
	}
}

func (m *Manager) tunnelID(name string) string {
	return stableID(m.Namespace, name)
}

func (m *Manager) withMasterLock(masterID string, run func() error) error {
	lockDir := filepath.Join(m.Dir, "locks")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(lockDir, masterID+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
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

func stableID(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)[:16])
}

func opensshMaster(master state.MasterRuntime) openssh.Master {
	return openssh.Master{Host: master.Host, SSHConfigPath: master.SSHConfigPath, ControlPath: master.ControlPath, LogPath: master.LogPath}
}
