package restoreverify

import (
	"context"

	"github.com/KazzyAPI/parachute/internal/backup"
)

type BackupSnapshotter struct {
	Inner *backup.Snapshotter
}

func (s BackupSnapshotter) ValidateTransaction(ctx context.Context, snapshot, host, stack, transaction string) error {
	return s.Inner.ValidateTransaction(ctx, snapshot, host, stack, transaction)
}

func (s BackupSnapshotter) Manifest(ctx context.Context, snapshot string) (backup.Manifest, error) {
	return s.Inner.Manifest(ctx, snapshot)
}

func (s BackupSnapshotter) RestoreToVolume(ctx context.Context, snapshot, archive, volume string) error {
	return s.Inner.RestoreToVolume(ctx, snapshot, archive, volume)
}

func (s BackupSnapshotter) BeginRestoreVerify(runID string) func() {
	return s.Inner.BeginRestoreVerify(runID)
}
