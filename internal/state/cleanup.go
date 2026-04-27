package state

import (
	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/platform"
)

func CleanupStale(path string) (model.State, []model.TunnelRuntime, error) {
	st, err := Load(path)
	if err != nil {
		return model.State{}, nil, err
	}
	var stale []model.TunnelRuntime
	out := st.Tunnels[:0]
	for _, entry := range st.Tunnels {
		if platform.ProcessMatches(entry.PID, entry.ProcessKey) {
			out = append(out, entry)
			continue
		}
		stale = append(stale, entry)
	}
	st.Tunnels = out
	if len(stale) > 0 {
		if err := Save(path, st); err != nil {
			return model.State{}, nil, err
		}
	}
	return st, stale, nil
}
