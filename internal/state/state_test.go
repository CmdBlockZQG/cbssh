package state

import (
	"path/filepath"
	"strconv"
	"sync"
	"testing"
)

func TestStoreUpdateRoundTrip(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "runtime.json"))
	if err := store.Update(func(registry *Registry) error {
		registry.Masters["master"] = MasterRuntime{ID: "master", Host: "prod"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	registry, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if registry.Version != 1 || registry.Masters["master"].Host != "prod" {
		t.Fatalf("unexpected registry: %#v", registry)
	}
}

func TestStoreSerializesConcurrentUpdates(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "runtime.json"))
	const updates = 20
	var wait sync.WaitGroup
	errors := make(chan error, updates)
	for i := 0; i < updates; i++ {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			errors <- store.Update(func(registry *Registry) error {
				id := strconv.Itoa(i)
				registry.Masters[id] = MasterRuntime{ID: id}
				return nil
			})
		}(i)
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	registry, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Masters) != updates {
		t.Fatalf("masters = %d, want %d", len(registry.Masters), updates)
	}
}
