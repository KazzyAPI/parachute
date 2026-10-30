package app

import (
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/hooks"
	"github.com/KazzyAPI/parachute/internal/txn"
)

func (c *Container) hooks(docker *engine.Docker) txn.Hooks {
	if !c.cfg.Hooks.Enabled {
		return txn.NoHooks{}
	}
	return hooks.New(c.labels, docker, docker, docker, c.redactor, c.logger)
}
