package restoreverify

import (
	"context"
	"strings"

	"github.com/moby/moby/api/types/mount"

	"github.com/KazzyAPI/parachute/internal/backup"
)

type stackRun struct {
	engine      Engine
	snapshotter Snapshotter
	target      Target
	manifest    backup.Manifest
	runID       string
	suffix      string
	volumes     map[string]string
}

func volumeKey(container, destination string) string {
	return container + "/" + destination
}

func (s *stackRun) restoreVolumes(ctx context.Context) string {
	s.volumes = map[string]string{}
	labels := ResourceLabels(s.runID)
	for _, container := range s.manifest.Containers {
		for _, record := range MountsForContainer(s.manifest, container.Name) {
			name := ThrowawayVolumeName(s.suffix, container.Name, record.Destination)
			if err := s.engine.CreateLabeledVolume(ctx, name, labels); err != nil {
				return reasonRestoreFailed
			}
			if err := s.snapshotter.RestoreToVolume(ctx, s.target.Snapshot.ID, record.File, name); err != nil {
				if strings.Contains(err.Error(), reasonSnapshotInvalid) {
					return reasonSnapshotInvalid
				}
				return reasonRestoreFailed
			}
			s.volumes[volumeKey(container.Name, record.Destination)] = name
		}
	}
	return ""
}

func (s *stackRun) createContainers(ctx context.Context) (map[string]string, string) {
	networkName := ThrowawayNetworkName(s.suffix)
	if err := s.engine.CreateInternalNetwork(ctx, networkName, ResourceLabels(s.runID)); err != nil {
		return nil, reasonRestoreFailed
	}
	ids := map[string]string{}
	for _, container := range s.manifest.Containers {
		image := ImageReference(container)
		if image == "" || s.engine.EnsureImage(ctx, image) != nil {
			return nil, reasonRestoreFailed
		}
		var mounts []mount.Mount
		for _, record := range MountsForContainer(s.manifest, container.Name) {
			mounts = append(mounts, mount.Mount{Type: mount.TypeVolume, Source: s.volumes[volumeKey(container.Name, record.Destination)], Target: record.Destination})
		}
		id, err := s.engine.Create(ctx, ThrowawaySpec(container, s.runID, s.suffix, networkName, ServiceAlias(container), mounts))
		if err != nil {
			return nil, reasonRestoreFailed
		}
		ids[container.Name] = id
	}
	return ids, ""
}
