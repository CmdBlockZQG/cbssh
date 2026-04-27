package hostview

import (
	"fmt"
	"sort"

	"github.com/cmdblock/cbssh/internal/model"
)

const (
	SortRecent = "recent"
	SortName   = "name"
)

// Sort returns a sorted copy of hosts so callers can keep config order intact.
func Sort(hosts []model.Host, st model.State, mode string) ([]model.Host, error) {
	out := append([]model.Host(nil), hosts...)
	switch mode {
	case "", SortRecent:
		sort.SliceStable(out, func(i, j int) bool {
			return st.Hosts[out[i].Name].LastUsed.After(st.Hosts[out[j].Name].LastUsed)
		})
	case SortName:
		sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	default:
		return nil, fmt.Errorf("unsupported sort mode %q", mode)
	}
	return out, nil
}
