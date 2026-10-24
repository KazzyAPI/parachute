package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/moby/moby/api/types/mount"
)

func (s *Snapshotter) Restore(ctx context.Context, snapshot string) error {
	return s.restore(ctx, snapshot, restoreOptions{rollback: true})
}

func (s *Snapshotter) RestoreToPath(ctx context.Context, snapshot, archive, targetPath string) error {
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
	return s.extractArchive(ctx, snapshot, archive, targetPath, data)
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
	snapshotDir := filepath.Join(s.hostStorePath(root), snapshot)
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
		target, err := hostPathForDestination(ctx, s.engine, spec, record.Destination)
		if err != nil {
			return err
		}
		if err := s.extractArchive(ctx, manifest.Transaction, record.File, target, archive); err != nil {
			return err
		}
		_ = snapshotDir
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

func (s *Snapshotter) extractArchive(ctx context.Context, transaction, file, targetPath string, archive []byte) error {
	if err := validateArchive(archive); err != nil {
		return fmt.Errorf("%s: %w", reasonSnapshotInvalid, err)
	}
	tmp, err := os.CreateTemp("", "parachute-restore-*.tar.gz")
	if err != nil {
		return err
	}
	path := tmp.Name()
	defer func() { _ = os.Remove(path) }()
	if _, err := tmp.Write(archive); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	mounts := []mount.Mount{
		{Type: mount.TypeBind, Source: targetPath, Target: "/target", ReadOnly: false},
		{Type: mount.TypeBind, Source: path, Target: "/archive.tar.gz", ReadOnly: true},
	}
	spec := engine.HelperSpec{
		Image:   s.settings.Helper,
		Command: []string{"sh", "-c", "gzip -dc /archive.tar.gz | tar -x -C /target"},
		Mounts:  mounts,
		Labels:  map[string]string{"parachute.helper": "true", "parachute.transaction": transaction},
		CapAdd:  []string{"CHOWN", "DAC_OVERRIDE", "FOWNER", "FSETID"},
	}
	id, err := s.engine.RunHelper(ctx, spec)
	if err != nil {
		return err
	}
	defer func() { _ = s.engine.Remove(ctx, id, false, true) }()
	code, err := s.engine.Wait(ctx, id)
	if err != nil || code != 0 {
		return fmt.Errorf("restore extract exit %d", code)
	}
	return nil
}

func (s *Snapshotter) hostStorePath(root storeRoot) string {
	if root.HostPath != "" {
		return root.HostPath
	}
	volume, err := s.engine.Volume(context.Background(), root.VolumeName)
	if err != nil {
		return ""
	}
	return volume.Mountpoint
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
