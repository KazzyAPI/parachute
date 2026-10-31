package app

import (
	"github.com/KazzyAPI/parachute/internal/graph"
	"github.com/KazzyAPI/parachute/internal/recreate"
	"github.com/KazzyAPI/parachute/internal/selfupdate"
	"github.com/KazzyAPI/parachute/internal/state"
	"github.com/KazzyAPI/parachute/internal/txn"
)

func (c *Container) selfRunner(store *state.Store, connected []connectedHost, helper string) graph.SelfRunner {
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
	var runner graph.SelfRunner = graph.NoSelfRunner{}
	for _, host := range connected {
		if host.docker == nil || host.err != nil {
			continue
		}
		snapshotter := c.hostSnapshotter(host, journal, helper, ids)
		runner = selfupdate.NewRunner(
			host.docker,
			journal,
			recordingSnapshotter{Snapshotter: snapshotter, journal: journal},
			recreate.NewRecreator(host.docker, recreateSettings),
			c.verifier(host.docker),
			c.deps.System,
			c.cfg.StateDir,
			ids,
			c.deps.Clock,
			c.logger,
			options,
			c.pullAuth,
		)
		break
	}
	return runner
}
