package hostview

import (
	"strings"
	"testing"
	"time"

	"github.com/cmdblock/cbssh/internal/model"
)

func TestSortByRecentReturnsCopy(t *testing.T) {
	hosts := []model.Host{{Name: "old"}, {Name: "new"}}
	st := model.State{Hosts: map[string]model.HostRuntime{
		"old": {LastUsed: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		"new": {LastUsed: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
	}}

	got, err := Sort(hosts, st, SortRecent)
	if err != nil {
		t.Fatalf("Sort returned error: %v", err)
	}
	if got[0].Name != "new" || got[1].Name != "old" {
		t.Fatalf("Sort order = %v, want new, old", hostNames(got))
	}
	if hosts[0].Name != "old" || hosts[1].Name != "new" {
		t.Fatalf("Sort mutated input = %v", hostNames(hosts))
	}
}

func TestSortRejectsUnsupportedMode(t *testing.T) {
	_, err := Sort(nil, model.State{}, "created")
	if err == nil || !strings.Contains(err.Error(), "unsupported sort mode") {
		t.Fatalf("Sort error = %v, want unsupported sort mode", err)
	}
}

func hostNames(hosts []model.Host) []string {
	names := make([]string, 0, len(hosts))
	for _, host := range hosts {
		names = append(names, host.Name)
	}
	return names
}
