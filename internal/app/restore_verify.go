package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/KazzyAPI/parachute/internal/backup"
	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/state"
	"github.com/KazzyAPI/parachute/internal/txn"
)

func (c *Container) RestoreVerify(ctx context.Context) error {
	store := state.NewStore(c.cfg.StateDir, c.deps.System, c.logger)
	document, ok, err := store.Read()
	if err != nil {
		return configurationError(err)
	}
	if !ok || len(document.Snapshots) == 0 {
		return errors.New("no snapshots to verify")
	}
	hosts, connected, closeHosts, err := c.hosts(ctx)
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
	_ = hosts
	host := connected[0]
	if host.docker == nil {
		return unreachableError(errors.New("no docker host"))
	}
	settings := backup.Settings{Mode: c.cfg.Backup.Mode, Volume: c.cfg.Backup.Volume, Retention: c.cfg.Backup.Retention, Helper: helper}
	snap := backup.NewSnapshotter(host.docker, settings, c.labels, txn.NewIDGenerator(c.deps.Clock.Now), c.deps.Clock, c.logger)
	latest := document.Snapshots[len(document.Snapshots)-1]
	manifest, err := snap.Manifest(ctx, latest.ID)
	if err != nil {
		return fmt.Errorf("restore verification failed: %w", err)
	}
	if len(manifest.Mounts) == 0 {
		return nil
	}
	record := manifest.Mounts[0]
	volumeName := "parachute-rv-" + latest.ID
	if err := host.docker.EnsureVolume(ctx, volumeName); err != nil {
		return err
	}
	if err := snap.RestoreToVolume(ctx, latest.ID, record.File, volumeName); err != nil {
		return &ExitError{Code: domain.ExitFailures, Err: fmt.Errorf("restore verification failed: %w", err)}
	}
	return nil
}
