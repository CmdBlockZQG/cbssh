package tunnel

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/CmdBlockZQG/cbssh/internal/model"
	"github.com/CmdBlockZQG/cbssh/internal/openssh"
	"github.com/CmdBlockZQG/cbssh/internal/state"
)

type fakeRunner struct {
	mu       sync.Mutex
	masters  map[string]bool
	forwards map[string]map[string]bool
	starts   int
	cancels  int
	exits    int
	failNext bool
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{masters: make(map[string]bool), forwards: make(map[string]map[string]bool)}
}

func (f *fakeRunner) StartMaster(_ context.Context, master openssh.Master) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.masters[master.ControlPath] {
		return errors.New("master already exists")
	}
	f.masters[master.ControlPath] = true
	f.forwards[master.ControlPath] = make(map[string]bool)
	f.starts++
	return nil
}

func (f *fakeRunner) CheckMaster(_ context.Context, master openssh.Master) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.masters[master.ControlPath], nil
}

func (f *fakeRunner) Forward(_ context.Context, master openssh.Master, flag, spec string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext {
		f.failNext = false
		return errors.New("forward failed")
	}
	if !f.masters[master.ControlPath] {
		return errors.New("master is not running")
	}
	f.forwards[master.ControlPath][flag+" "+spec] = true
	return nil
}

func (f *fakeRunner) Cancel(_ context.Context, master openssh.Master, flag, spec string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := flag + " " + spec
	if !f.forwards[master.ControlPath][key] {
		return errors.New("forward does not exist")
	}
	delete(f.forwards[master.ControlPath], key)
	f.cancels++
	return nil
}

func (f *fakeRunner) Exit(_ context.Context, master openssh.Master) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.masters[master.ControlPath] {
		return errors.New("master is not running")
	}
	delete(f.masters, master.ControlPath)
	delete(f.forwards, master.ControlPath)
	f.exits++
	return nil
}

func (f *fakeRunner) ValidateHost(context.Context, string, string) error { return nil }

func TestSameHostSharesMasterUntilLastTunnelStops(t *testing.T) {
	manager, runner := testManager(t)
	first := localTunnel("db", "prod", 15432, 5432)
	second := dynamicTunnel("socks", "prod", 1080)

	firstResult, err := manager.Start(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	secondResult, err := manager.Start(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	if runner.starts != 1 {
		t.Fatalf("master starts = %d, want 1", runner.starts)
	}
	if firstResult.MasterID != secondResult.MasterID {
		t.Fatalf("master IDs differ: %s and %s", firstResult.MasterID, secondResult.MasterID)
	}

	if _, err := manager.Stop(context.Background(), first.Name); err != nil {
		t.Fatal(err)
	}
	if runner.cancels != 1 || runner.exits != 0 {
		t.Fatalf("after first stop cancels=%d exits=%d, want 1/0", runner.cancels, runner.exits)
	}
	if _, err := manager.Stop(context.Background(), second.Name); err != nil {
		t.Fatal(err)
	}
	if runner.exits != 1 {
		t.Fatalf("master exits = %d, want 1", runner.exits)
	}
}

func TestDifferentHostsUseDifferentMasters(t *testing.T) {
	manager, runner := testManager(t)
	if _, err := manager.Start(context.Background(), localTunnel("one", "prod-a", 10001, 80)); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Start(context.Background(), localTunnel("two", "prod-b", 10002, 80)); err != nil {
		t.Fatal(err)
	}
	if runner.starts != 2 {
		t.Fatalf("master starts = %d, want 2", runner.starts)
	}
}

func TestForwardFailureKeepsExistingSharedMaster(t *testing.T) {
	manager, runner := testManager(t)
	if _, err := manager.Start(context.Background(), localTunnel("one", "prod", 10001, 80)); err != nil {
		t.Fatal(err)
	}
	runner.failNext = true
	if _, err := manager.Start(context.Background(), localTunnel("two", "prod", 10002, 80)); err == nil {
		t.Fatal("Start() error = nil, want forward failure")
	}
	if runner.starts != 1 || runner.exits != 0 {
		t.Fatalf("starts=%d exits=%d, want existing master preserved", runner.starts, runner.exits)
	}
	names, err := manager.ActiveNames(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "one" {
		t.Fatalf("active names = %#v, want [one]", names)
	}
}

func TestFirstForwardFailureExitsNewMaster(t *testing.T) {
	manager, runner := testManager(t)
	runner.failNext = true
	if _, err := manager.Start(context.Background(), localTunnel("one", "prod", 10001, 80)); err == nil {
		t.Fatal("Start() error = nil, want forward failure")
	}
	if runner.starts != 1 || runner.exits != 1 {
		t.Fatalf("starts=%d exits=%d, want new master rolled back", runner.starts, runner.exits)
	}
	names, err := manager.ActiveNames(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Fatalf("active names = %#v, want none", names)
	}
}

func TestDeadMasterRemovesAllSharedTunnels(t *testing.T) {
	manager, runner := testManager(t)
	if _, err := manager.Start(context.Background(), localTunnel("one", "prod", 10001, 80)); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Start(context.Background(), dynamicTunnel("two", "prod", 1080)); err != nil {
		t.Fatal(err)
	}
	runner.mu.Lock()
	for controlPath := range runner.masters {
		delete(runner.masters, controlPath)
		delete(runner.forwards, controlPath)
	}
	runner.mu.Unlock()

	names, err := manager.ActiveNames(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Fatalf("active names = %#v, want none after master death", names)
	}
}

func TestStatusMarksChangedDefinition(t *testing.T) {
	manager, _ := testManager(t)
	definition := localTunnel("one", "prod", 10001, 80)
	if _, err := manager.Start(context.Background(), definition); err != nil {
		t.Fatal(err)
	}
	definition.TargetPort = 81
	rows, err := manager.Status(context.Background(), []model.Tunnel{definition}, []string{"one"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].State != "changed" {
		t.Fatalf("status rows = %#v, want changed", rows)
	}
}

func TestReconcileRemovesTunnelWithoutMaster(t *testing.T) {
	manager, _ := testManager(t)
	definition := localTunnel("one", "prod", 10001, 80)
	if err := manager.Store.Update(func(registry *state.Registry) error {
		id := manager.tunnelID(definition.Name)
		registry.Tunnels[id] = state.TunnelRuntime{ID: id, Namespace: manager.Namespace, Name: definition.Name, MasterID: "missing", Definition: definition, Phase: state.PhaseActive}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	names, err := manager.ActiveNames(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Fatalf("active names = %#v, want orphan removed", names)
	}
}

func testManager(t *testing.T) (*Manager, *fakeRunner) {
	t.Helper()
	dir := t.TempDir()
	runner := newFakeRunner()
	manager := NewManager(Options{
		ConfigPath: filepath.Join(dir, "tunnels.toml"),
		StatePath:  filepath.Join(dir, "runtime.json"),
		Runner:     runner,
	})
	return manager, runner
}

func localTunnel(name, host string, bindPort, targetPort int) model.Tunnel {
	return model.Tunnel{Name: name, Host: host, Type: model.TunnelTypeLocal, BindHost: "127.0.0.1", BindPort: bindPort, TargetHost: "127.0.0.1", TargetPort: targetPort}
}

func dynamicTunnel(name, host string, bindPort int) model.Tunnel {
	return model.Tunnel{Name: name, Host: host, Type: model.TunnelTypeDynamic, BindHost: "127.0.0.1", BindPort: bindPort}
}
