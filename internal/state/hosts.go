package state

import (
	"time"

	"github.com/cmdblock/cbssh/internal/model"
)

func MarkHostUsed(path string, name string, usedAt time.Time) error {
	st, err := Load(path)
	if err != nil {
		return err
	}
	if st.Hosts == nil {
		st.Hosts = map[string]model.HostRuntime{}
	}
	st.Hosts[name] = model.HostRuntime{LastUsed: usedAt}
	return Save(path, st)
}
