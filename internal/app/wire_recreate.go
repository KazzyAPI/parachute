package app

import (
	"time"

	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/recreate"
	"github.com/KazzyAPI/parachute/internal/state"
	"github.com/KazzyAPI/parachute/internal/txn"
)

func (c *Container) updateStage(store *state.Store, hosts []pipeline.Host, dockers []*engine.Docker) pipeline.Stage {
	engines := map[string]recreate.Engine{}
	for index, host := range hosts {
		if index < len(dockers) && dockers[index] != nil {
			engines[host.Name] = dockers[index]
		}
	}
	local := engines["local"]
	if local == nil {
		for _, engine := range engines {
			local = engine
			break
		}
	}
	settings := func() recreate.Settings {
		return recreate.Settings{RemoveVolumes: c.cfg.Update.RemoveVolumes, CommittedAt: c.deps.Clock.Now()}
	}
	recreator := recreate.NewRecreator(local, settings)
	ids := txn.NewIDGenerator(c.deps.Clock.Now)
	return recreate.NewStage(
		engines,
		recreator,
		recreate.NoopSnapshotter{},
		recreate.NewHealthVerifier(local, c.deps.Clock, 15*time.Second),
		ids,
		c.labels,
		store,
		c.cfg.Update.StopTimeout,
		c.cfg.Update.Cleanup,
		c.cfg.Update.RemoveVolumes,
		c.logger,
		c.deps.Clock.Now,
	)
}
