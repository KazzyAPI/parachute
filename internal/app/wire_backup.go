package app

import (
	"context"
	"errors"

	"github.com/KazzyAPI/parachute/internal/backup"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/state"
	"github.com/KazzyAPI/parachute/internal/txn"
)

func (c *Container) requireHelperImage(ctx context.Context, hosts []connectedHost) (string, error) {
	if c.cfg.Backup.Helper != "" {
		return c.cfg.Backup.Helper, nil
	}
	self := c.self()
	for _, host := range hosts {
		if host.err != nil {
			continue
		}
		containers, err := host.docker.Containers(ctx)
		if err != nil {
			continue
		}
		if self.Mark([]engine.SelfHost{{Local: engine.LocalEndpoint(host.spec.Endpoint), Containers: containers}}) {
			for _, container := range containers {
				if container.Own {
					return container.ImageRef, nil
				}
			}
		}
	}
	if !self.Contained() && c.cfg.Armed {
		return "", errors.New("helper-image is required when not containerised and armed")
	}
	return c.cfg.Backup.Helper, nil
}

func (c *Container) executeStage(store *state.Store, hosts []connectedHost, helper string) pipeline.Stage {
	journal := state.NewJournal(store, c.deps.Clock.Now)
	engines := map[string]*engine.Docker{}
	settings := backup.Settings{
		Mode: c.cfg.Backup.Mode, Volume: c.cfg.Backup.Volume, Retention: c.cfg.Backup.Retention, Helper: helper,
	}
	ids := txn.NewIDGenerator(c.deps.Clock.Now)
	snapshotters := map[string]txn.Snapshotter{}
	for _, host := range hosts {
		if host.docker == nil {
			continue
		}
		engines[host.spec.Name] = host.docker
		snapshotters[host.spec.Name] = backup.NewSnapshotter(host.docker, settings, c.labels, ids, c.deps.Clock, c.logger)
	}
	executor := &stackExecutor{
		engines: engines, ids: ids, labels: c.labels,
		cfg: updateSettings{
			stopTimeout: c.cfg.Update.StopTimeout, verifyTimeout: c.cfg.Update.VerifyTimeout,
			verifyStable: c.cfg.Update.VerifyStable, helperImage: helper,
		},
		logger: c.logger,
	}
	return NewExecuteStage(c.cfg.Armed, journal, snapshotters, executor, c.logger)
}
