package state

import "github.com/cmdblock/cbssh/internal/model"

func Empty() model.State {
	return model.State{
		Version: 1,
		Hosts:   map[string]model.HostRuntime{},
	}
}
