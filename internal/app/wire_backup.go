package app

import (
	"context"
	"errors"

	"github.com/KazzyAPI/parachute/internal/backup"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/state"
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
