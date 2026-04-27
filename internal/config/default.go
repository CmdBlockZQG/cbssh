package config

import "github.com/cmdblock/cbssh/internal/model"

func Empty() model.Config {
	cfg := model.Config{}
	cfg.Normalize()
	return cfg
}
