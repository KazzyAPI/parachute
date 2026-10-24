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
	snapshotters := map[string]*backup.Snapshotter{}
	settings := backup.Settings{
		Mode: c.cfg.Backup.Mode, Volume: c.cfg.Backup.Volume, Retention: c.cfg.Backup.Retention, Helper: helper,
	}
	ids := txn.NewIDGenerator(c.deps.Clock.Now)
	for _, host := range hosts {
		if host.docker == nil {
			continue
		}
		engines[host.spec.Name] = host.docker
		snap := backup.NewSnapshotter(host.docker, settings, c.labels, ids, c.deps.Clock, c.logger)
		snap.SetCatalog(catalogFor(journal))
		snapshotters[host.spec.Name] = snap
	}
	executor := &stackExecutor{
		engines: engines, snapshotters: snapshotters, ids: ids, labels: c.labels,
		cfg: updateSettings{
			stopTimeout: c.cfg.Update.StopTimeout, verifyTimeout: c.cfg.Verify.Timeout,
			verifyStable: c.cfg.Verify.Stable, helperImage: helper, backupDefault: c.cfg.Backup.Mode,
		},
		logger: c.logger, clock: c.deps.Clock,
	}
	return NewExecuteStage(c.cfg.Armed, journal, snapshotters, c.cfg.Backup.Retention, executor, c.logger)
}

func catalogFor(journal *state.Journal) backup.SnapshotCatalog {
	return func(host, stack string) []backup.RecordedSnapshot {
		var snapshots []backup.RecordedSnapshot
		for _, record := range journal.Snapshots(host, stack) {
			snapshots = append(snapshots, backup.RecordedSnapshot{ID: record.ID, Transaction: record.Transaction, Time: record.Time})
		}
		return snapshots
	}
}

func (c *Container) cleanupHelpers(ctx context.Context, hosts []connectedHost, journal *state.Journal) {
	allowed := journal.OpenTransactionIDs()
	for _, host := range hosts {
		if host.docker == nil {
			continue
		}
		_ = backup.CleanupHelpers(ctx, host.docker, allowed)
	}
}
