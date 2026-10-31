package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/KazzyAPI/parachute/internal/backup"
	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/restoreverify"
	"github.com/KazzyAPI/parachute/internal/state"
	"github.com/KazzyAPI/parachute/internal/txn"
	"github.com/KazzyAPI/parachute/internal/verify"
)

func (c *Container) RestoreVerify(ctx context.Context) error {
	_, connected, closeHosts, err := c.hosts(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = closeHosts() }()
	helper, err := c.requireHelperImage(ctx, connected)
	if err != nil {
		return configurationError(err)
	}
	if helper == "" {
		return configurationError(errors.New("helper-image is required for restore-verify when not containerised"))
	}
	store := state.NewStore(c.cfg.StateDir, c.deps.System, c.logger)
	document, ok, err := store.Read()
	if err != nil {
		return configurationError(err)
	}
	if !ok {
		document = state.Document{}
	}
	catalog := snapshotCatalog(document)
	engines := map[string]restoreverify.Engine{}
	snapshotters := map[string]restoreverify.Snapshotter{}
	ids := txn.NewIDGenerator(c.deps.Clock.Now)
	settings := backup.Settings{Mode: c.cfg.Backup.Mode, Volume: c.cfg.Backup.Volume, Retention: c.cfg.Backup.Retention, Helper: helper, StopTimeout: c.cfg.Update.StopTimeout}
	for _, host := range connected {
		if host.docker == nil || host.err != nil {
			continue
		}
		engines[host.spec.Name] = host.docker
		inner := backup.NewSnapshotter(host.docker, settings, c.labels, ids, c.deps.Clock, c.logger)
		inner.SetCatalog(catalog)
		snapshotters[host.spec.Name] = restoreverify.BackupSnapshotter{Inner: inner}
	}
	if len(engines) == 0 {
		return unreachableError(errors.New("no docker host"))
	}
	hostFilter := c.cfg.RestoreVerify.Host
	if hostFilter == "" && len(c.hostSpecs()) == 1 {
		hostFilter = c.hostSpecs()[0].Name
	}
	options := restoreverify.Options{
		StateDir:  c.cfg.StateDir,
		Host:      hostFilter,
		Stack:     c.cfg.RestoreVerify.Stack,
		Retention: c.cfg.Backup.Retention,
	}
	verifier := restoreHostVerifier{hosts: map[string]txn.Verifier{}}
	for _, host := range connected {
		if host.docker == nil || host.err != nil {
			continue
		}
		verifier.hosts[host.spec.Name] = c.restoreVerifyVerifier(host.docker, helper)
	}
	runner := restoreverify.NewRunner(c.deps.System, snapshotters, verifier, ids, c.deps.Clock, c.logger)
	outcome := runner.Run(ctx, engines, document, options)
	if outcome.Err == nil {
		return nil
	}
	if outcome.ExitCode == domain.ExitFailures {
		return &ExitError{Code: domain.ExitFailures, Err: outcome.Err}
	}
	return configurationError(outcome.Err)
}

func snapshotCatalog(document state.Document) backup.SnapshotCatalog {
	return func(host, stack string) []backup.RecordedSnapshot {
		var snapshots []backup.RecordedSnapshot
		for _, record := range document.Snapshots {
			if record.Host != host || record.Stack != stack {
				continue
			}
			snapshots = append(snapshots, backup.RecordedSnapshot{ID: record.ID, Transaction: record.Transaction, Time: record.Time})
		}
		return snapshots
	}
}

func (c *Container) restoreVerifyVerifier(docker *engine.Docker, helper string) txn.Verifier {
	settings := verify.Settings{Timeout: c.cfg.Verify.Timeout, Stable: c.cfg.Verify.Stable}
	labels := verify.NewLabelAdapter(c.labels)
	prober := &urlProber{docker: docker, helperImage: helper, ownerLabel: "parachute.restore-verify"}
	return verify.New(docker, docker, prober, labels, settings, c.deps.Clock)
}

func (c *Container) cleanupRestoreVerify(ctx context.Context, connected []connectedHost) {
	engines := map[string]restoreverify.Engine{}
	for _, host := range connected {
		if host.docker != nil && host.err == nil {
			engines[host.spec.Name] = host.docker
		}
	}
	if len(engines) == 0 {
		return
	}
	_ = restoreverify.NewRecoverer(c.deps.System, c.deps.Clock).StartupCleanup(ctx, engines, c.cfg.StateDir)
}

type restoreHostVerifier struct {
	hosts map[string]txn.Verifier
}

func (v restoreHostVerifier) Verify(ctx context.Context, plan txn.Plan, containerID string) (txn.Verdict, error) {
	verifier := v.hosts[plan.Host]
	if verifier == nil {
		return txn.Verdict{}, fmt.Errorf("no verifier for host %s", plan.Host)
	}
	return verifier.Verify(ctx, plan, containerID)
}
