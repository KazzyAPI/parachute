package backup

import (
	"context"
	"fmt"

	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/moby/moby/api/types/mount"
)

func (s *Snapshotter) Restore(ctx context.Context, snapshot string) error {
	return s.restore(ctx, snapshot, restoreOptions{rollback: true})
}

func (s *Snapshotter) RestoreToVolume(ctx context.Context, snapshot, archive, volume string) error {
	root, err := s.store.Root(ctx)
	if err != nil {
		return err
	}
	if err := validateSnapshot(ctx, s.store, root, snapshot, "", "", ""); err != nil {
		return fmt.Errorf("%s: %w", reasonSnapshotInvalid, err)
	}
	data, err := s.store.ReadFile(ctx, root, snapshot, archive)
	if err != nil {
		return err
	}
	if err := validateArchive(data); err != nil {
		return fmt.Errorf("%s: %w", reasonSnapshotInvalid, err)
	}
	target := mount.Mount{Type: mount.TypeVolume, Source: volume, Target: "/target"}
	return s.extractArchive(ctx, root, snapshot, snapshot, archive, target, data)
}

type restoreOptions struct {
	rollback bool
}

func (s *Snapshotter) restore(ctx context.Context, snapshot string, options restoreOptions) error {
	root, err := s.store.Root(ctx)
	if err != nil {
		return err
	}
	if err := validateSnapshot(ctx, s.store, root, snapshot, "", "", ""); err != nil {
		return fmt.Errorf("%s: %w", reasonSnapshotInvalid, err)
	}
	data, err := s.store.ReadFile(ctx, root, snapshot, "manifest.json")
	if err != nil {
		return err
	}
	manifest, err := decodeManifest(data)
	if err != nil {
		return fmt.Errorf("%s: %w", reasonSnapshotInvalid, err)
	}
	for _, record := range manifest.Mounts {
		archive, err := s.store.ReadFile(ctx, root, snapshot, record.File)
		if err != nil {
			return err
		}
		if err := validateArchive(archive); err != nil {
			return fmt.Errorf("%s: %w", reasonSnapshotInvalid, err)
		}
		spec, err := s.containerSpec(ctx, record.Container)
		if err != nil {
			return err
		}
		target, err := mountForDestination(spec, record.Destination)
		if err != nil {
			return err
		}
		if err := s.extractArchive(ctx, root, snapshot, manifest.Transaction, record.File, target, archive); err != nil {
			return err
		}
	}
	return nil
}

func (s *Snapshotter) containerSpec(ctx context.Context, name string) (engine.Spec, error) {
	containers, err := s.engine.Containers(ctx)
	if err != nil {
		return engine.Spec{}, err
	}
	for _, container := range containers {
		if container.Name != name {
			continue
		}
		return s.engine.Inspect(ctx, container.ID)
	}
	return engine.Spec{}, fmt.Errorf("container %s not found", name)
}

func (s *Snapshotter) extractArchive(ctx context.Context, root storeRoot, snapshot, transaction, file string, target mount.Mount, archive []byte) error {
	if err := validateArchive(archive); err != nil {
		return fmt.Errorf("%s: %w", reasonSnapshotInvalid, err)
	}
	if err := entryPath(snapshot, file); err != nil {
		return err
	}
	s.run.txn = transaction
	if _, err := s.run.run(ctx, []mount.Mount{root.mount(true), target}, smallOutput,
		`gzip -dc "/snap/$1/$2" | tar -x -C /target`, snapshot, file); err != nil {
		return fmt.Errorf("restore extract: %w", err)
	}
	return nil
}

func (s *Snapshotter) SkippedFromSnapshot(ctx context.Context, snapshot string) ([]SkippedRecord, error) {
	root, err := s.store.Root(ctx)
	if err != nil {
		return nil, err
	}
	data, err := s.store.ReadFile(ctx, root, snapshot, "manifest.json")
	if err != nil {
		return nil, err
	}
	manifest, err := decodeManifest(data)
	if err != nil {
		return nil, err
	}
	return manifest.Skipped, nil
}

func (s *Snapshotter) ValidateTransaction(ctx context.Context, snapshot, host, stack, transaction string) error {
	root, err := s.store.Root(ctx)
	if err != nil {
		return err
	}
	return validateSnapshot(ctx, s.store, root, snapshot, host, stack, transaction)
}

func (s *Snapshotter) Manifest(ctx context.Context, snapshot string) (Manifest, error) {
	root, err := s.store.Root(ctx)
	if err != nil {
		return Manifest{}, err
	}
	data, err := s.store.ReadFile(ctx, root, snapshot, "manifest.json")
	if err != nil {
		return Manifest{}, err
	}
	return decodeManifest(data)
}
