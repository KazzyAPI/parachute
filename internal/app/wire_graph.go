package app

import (
	"context"

	"github.com/KazzyAPI/parachute/internal/backup"
	"github.com/KazzyAPI/parachute/internal/graph"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/recreate"
	"github.com/KazzyAPI/parachute/internal/state"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type recordingSnapshotter struct {
	*backup.Snapshotter
	journal *state.Journal
}

func (r recordingSnapshotter) Take(ctx context.Context, plan txn.Plan) (string, error) {
	snapshot, err := r.Snapshotter.Take(ctx, plan)
	if err != nil {
		return "", err
	}
	for _, stack := range plan.Stacks {
		if err := r.journal.RecordSnapshot(plan.Host, stack, plan.ID, snapshot); err != nil {
			return "", err
		}
	}
	return snapshot, nil
}

func (c *Container) graphStage(store *state.Store, connected []connectedHost, helper string) pipeline.Stage {
	journal := state.NewJournal(store, c.deps.Clock.Now)
	ids := txn.NewIDGenerator(c.deps.Clock.Now)
	settings := backup.Settings{Mode: c.cfg.Backup.Mode, Volume: c.cfg.Backup.Volume, Retention: c.cfg.Backup.Retention, Helper: helper}
	options := graph.Options{
		StopTimeout:   c.cfg.Update.StopTimeout,
		Rolling:       c.cfg.Update.RollingRestart,
		Cleanup:       c.cfg.Update.Cleanup,
		RemoveVolumes: c.cfg.Update.RemoveVolumes,
		Retention:     c.cfg.Backup.Retention,
	}
	recreateSettings := func() recreate.Settings {
		return recreate.Settings{RemoveVolumes: c.cfg.Update.RemoveVolumes, CommittedAt: c.deps.Clock.Now()}
	}
	executors := map[string]*graph.Executor{}
	for _, host := range connected {
		if host.docker == nil || host.err != nil {
			continue
		}
		snapshotter := backup.NewSnapshotter(host.docker, settings, c.labels, ids, c.deps.Clock, c.logger)
		snapshotter.SetCatalog(catalogFor(journal))
		executors[host.spec.Name] = graph.NewExecutor(
			graph.NewBuilder(),
			host.docker,
			journal,
			recordingSnapshotter{Snapshotter: snapshotter, journal: journal},
			recreate.NewRecreator(host.docker, recreateSettings),
			c.verifier(host.docker),
			ids,
			c.deps.Clock,
			c.logger,
			options,
			"",
		)
	}
	return graph.NewStage(executors, "")
}
