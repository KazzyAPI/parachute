package app

import (
	"context"

	"github.com/KazzyAPI/parachute/internal/backup"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/graph"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/recreate"
	"github.com/KazzyAPI/parachute/internal/registry"
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
	if snapshot == backup.NoSnapshot {
		return snapshot, nil
	}
	for _, stack := range plan.Stacks {
		if err := r.journal.RecordSnapshot(plan.Host, stack, plan.ID, snapshot); err != nil {
			return "", err
		}
	}
	return snapshot, nil
}

func (c *Container) hostSnapshotter(host connectedHost, journal *state.Journal, helper string, ids *txn.IDGenerator) *backup.Snapshotter {
	settings := backup.Settings{Mode: c.cfg.Backup.Mode, Volume: c.cfg.Backup.Volume, Retention: c.cfg.Backup.Retention, Helper: helper}
	snapshotter := backup.NewSnapshotter(host.docker, settings, c.labels, ids, c.deps.Clock, c.logger)
	snapshotter.SetCatalog(catalogFor(journal))
	return snapshotter
}

func (c *Container) hostFinisher(host connectedHost, snapshotter *backup.Snapshotter) *RollbackFinisher {
	return NewRollbackFinisher(host.docker, snapshotter, c.verifier(host.docker), c.labels, c.logger)
}

func (c *Container) graphStages(store *state.Store, connected []connectedHost, helper string, selfRunner graph.SelfRunner) (pipeline.Stage, pipeline.Stage) {
	journal := state.NewJournal(store, c.deps.Clock.Now)
	ids := txn.NewIDGenerator(c.deps.Clock.Now)
	options := graph.Options{
		StopTimeout:        c.cfg.Update.StopTimeout,
		Rolling:            c.cfg.Update.RollingRestart,
		Cleanup:            c.cfg.Update.Cleanup,
		RemoveVolumes:      c.cfg.Update.RemoveVolumes,
		Retention:          c.cfg.Backup.Retention,
		TransactionTimeout: c.cfg.Update.TransactionTimeout,
		NoRestart:          c.cfg.NoRestart,
		ReviveStopped:      c.cfg.Update.ReviveStopped,
	}
	recreateSettings := func() recreate.Settings {
		return recreate.Settings{RemoveVolumes: c.cfg.Update.RemoveVolumes, CommittedAt: c.deps.Clock.Now()}
	}
	executors := map[string]*graph.Executor{}
	for _, host := range connected {
		if host.docker == nil || host.err != nil {
			continue
		}
		snapshotter := c.hostSnapshotter(host, journal, helper, ids)
		executors[host.spec.Name] = graph.NewExecutor(
			graph.NewBuilder(),
			host.docker,
			journal,
			recordingSnapshotter{Snapshotter: snapshotter, journal: journal},
			recreate.NewRecreator(host.docker, recreateSettings),
			c.verifier(host.docker),
			c.hostFinisher(host, snapshotter),
			txn.NoHooks{},
			selfRunner,
			ids,
			c.deps.Clock,
			c.logger,
			options,
			c.pullAuth,
		)
	}
	return graph.NewStage(executors, c.ownContainerName(connected)), graph.NewFinishStage(selfRunner)
}

func (c *Container) ownContainerName(connected []connectedHost) string {
	self := c.self()
	for _, host := range connected {
		if host.docker == nil || host.err != nil {
			continue
		}
		containers, err := host.docker.Containers(context.Background())
		if err != nil {
			continue
		}
		if self.Mark([]engine.SelfHost{{Local: engine.LocalEndpoint(host.spec.Endpoint), Containers: containers}}) {
			for _, container := range containers {
				if container.Own {
					return container.Name
				}
			}
		}
	}
	return ""
}

func (c *Container) recovery(connected []connectedHost, journal *state.Journal, helper string) *Recovery {
	ids := txn.NewIDGenerator(c.deps.Clock.Now)
	hosts := map[string]RecoveryHost{}
	for _, host := range connected {
		if host.docker == nil || host.err != nil {
			continue
		}
		hosts[host.spec.Name] = RecoveryHost{Lister: host.docker, Finisher: c.hostFinisher(host, c.hostSnapshotter(host, journal, helper, ids))}
	}
	return NewRecovery(hosts, journal, c.logger)
}

func (c *Container) pullAuth(ref registry.Reference) engine.RegistryAuth {
	credentials, err := registry.NewCredentialStore(c.cfg.Detection.DockerConfigDir, c.deps.System).Load()
	if err != nil {
		return ""
	}
	return engine.RegistryAuth(credentials.PullAuth(ref))
}
