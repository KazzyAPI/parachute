package app

import (
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/hooks"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/txn"
)

func (c *Container) hookStages(connected []connectedHost) (pipeline.Stage, pipeline.Stage) {
	byHost := map[string]txn.Hooks{}
	for _, host := range connected {
		if host.docker == nil || host.err != nil {
			continue
		}
		byHost[host.spec.Name] = c.hooks(host.docker)
	}
	return pipeline.NewPreCheckStage(byHost), pipeline.NewPostCheckStage(byHost)
}

func (c *Container) hooks(docker *engine.Docker) txn.Hooks {
	if !c.cfg.Hooks.Enabled {
		return txn.NoHooks{}
	}
	return hooks.New(c.labels, docker, docker, docker, c.redactor, c.logger)
}
