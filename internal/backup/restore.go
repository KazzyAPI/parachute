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
	s.run.restoreVerify = snapshot
	defer func() { s.run.restoreVerify = "" }()
	return s.extractArchive(ctx, root, snapshot, "", archive, target)
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
	skipped := skippedByContainer(manifest)
	targets := make([]mount.Mount, len(manifest.Mounts))
	for index, record := range manifest.Mounts {
		if destinations, ok := skipped[record.Container]; ok && destinations[record.Destination] {
			return fmt.Errorf("%s: skipped mount %s", reasonSnapshotInvalid, record.Destination)
		}
		archive, err := s.store.ReadFile(ctx, root, snapshot, record.File)
		if err != nil {
			return err
		}
		if err := validateArchive(archive); err != nil {
			return fmt.Errorf("%s: %w", reasonSnapshotInvalid, err)
		}
		spec, err := s.containerSpec(ctx, record.Container, manifest.Transaction)
		if err != nil {
			return err
		}
		target, err := mountForDestination(spec, record.Destination)
		if err != nil {
			return err
		}
		if err := s.guardRestoreTarget(ctx, target); err != nil {
			return err
		}
		targets[index] = target
	}
	for index, record := range manifest.Mounts {
		if err := s.extractArchive(ctx, root, snapshot, manifest.Transaction, record.File, targets[index]); err != nil {
			return err
		}
	}
	return nil
}

func skippedByContainer(manifest Manifest) map[string]map[string]bool {
	byContainer := map[string]map[string]bool{}
	for _, record := range manifest.Skipped {
		if byContainer[record.Container] == nil {
			byContainer[record.Container] = map[string]bool{}
		}
		byContainer[record.Container][record.Destination] = true
	}
	return byContainer
}

func (s *Snapshotter) guardRestoreTarget(ctx context.Context, target mount.Mount) error {
	if target.Type != mount.TypeBind {
		return nil
	}
	resolved, err := s.hostFS.Resolve(ctx, target.Source)
	if err != nil {
		return err
	}
	if systemMount(resolved.Path, s.settings.Volume) {
		return fmt.Errorf("%s: restore into system mount %s", reasonSnapshotInvalid, resolved.Path)
	}
	return nil
}

func renamedOriginal(name, transaction string) string {
	if len(transaction) < 6 {
		return name
	}
	return name + "-parachute-" + transaction[len(transaction)-6:]
}

func (s *Snapshotter) containerSpec(ctx context.Context, name, transaction string) (engine.Spec, error) {
	containers, err := s.engine.Containers(ctx)
	if err != nil {
		return engine.Spec{}, err
	}
	for _, container := range containers {
		if container.Name != name && container.Name != renamedOriginal(name, transaction) {
			continue
		}
		return s.engine.Inspect(ctx, container.ID)
	}
	return engine.Spec{}, fmt.Errorf("container %s not found", name)
}

const extractScript = `find /target -mindepth 1 -maxdepth 1 -exec rm -rf {} + && gzip -dc "/snap/$1/$2" | tar -x -C /target`

func (s *Snapshotter) extractArchive(ctx context.Context, root storeRoot, snapshot, transaction, file string, target mount.Mount) error {
	if err := entryPath(snapshot, file); err != nil {
		return err
	}
	s.run.txn = transaction
	if _, err := s.run.run(ctx, []mount.Mount{root.mount(true), target}, smallOutput,
		extractScript, snapshot, file); err != nil {
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

func (s *Snapshotter) SkippedDestinations(ctx context.Context, snapshot string) (map[string][]string, error) {
	records, err := s.SkippedFromSnapshot(ctx, snapshot)
	if err != nil {
		return nil, err
	}
	byContainer := map[string][]string{}
	for _, record := range records {
		byContainer[record.Container] = append(byContainer[record.Container], record.Destination)
	}
	return byContainer, nil
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
