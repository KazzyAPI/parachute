package backup

import (
	"context"
	"strings"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/moby/moby/api/types/mount"
)

func planMounts(ctx context.Context, engine Engine, hostFS HostFS, labels LabelReader, storePath string, container domain.Container, spec engine.Spec, outsiders map[string]bool) ([]plannedMount, []SkippedRecord, error) {
	excludes := excludeSet(labels, container)
	var planned []plannedMount
	var skipped []SkippedRecord
	for _, item := range specMounts(spec) {
		if item.ReadOnly || item.Type == mount.TypeTmpfs {
			continue
		}
		if excludes[item.Target] {
			continue
		}
		source, src, skip, err := mountSource(ctx, engine, hostFS, storePath, item)
		if err != nil {
			return nil, nil, err
		}
		if skip {
			skipped = append(skipped, SkippedRecord{Container: container.Name, Destination: item.Target, Source: source})
			continue
		}
		consistency := "stopped"
		if outsiders[item.Source] {
			consistency = "live"
		}
		planned = append(planned, plannedMount{
			Container: container.Name, Destination: item.Target, Source: source, Kind: item.Type,
			VolumeName: item.Source, Consistency: consistency, SrcMount: src,
		})
	}
	return planned, skipped, nil
}

func excludeSet(labels LabelReader, container domain.Container) map[string]bool {
	raw, ok := labels.Value(container, "backup.exclude")
	if !ok {
		return nil
	}
	set := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			set[part] = true
		}
	}
	return set
}

func mountSource(ctx context.Context, engine Engine, hostFS HostFS, storePath string, item mount.Mount) (string, mount.Mount, bool, error) {
	switch item.Type {
	case mount.TypeVolume:
		if item.Source == storePath {
			return item.Source, mount.Mount{}, true, nil
		}
		volume, err := engine.Volume(ctx, item.Source)
		if err != nil {
			return "", mount.Mount{}, false, err
		}
		if device := volume.Options["device"]; device != "" {
			resolved, err := hostFS.Resolve(ctx, device)
			if err != nil {
				return "", mount.Mount{}, false, err
			}
			if systemMount(resolved.Path, storePath) {
				return resolved.Path, mount.Mount{}, true, nil
			}
		}
		return volume.Mountpoint, mount.Mount{Type: mount.TypeVolume, Source: item.Source, Target: "/src", ReadOnly: true}, false, nil
	case mount.TypeBind:
		resolved, err := hostFS.Resolve(ctx, item.Source)
		if err != nil {
			return "", mount.Mount{}, false, err
		}
		if !resolved.Backable {
			return item.Source, mount.Mount{}, true, nil
		}
		if systemMount(resolved.Path, storePath) {
			return resolved.Path, mount.Mount{}, true, nil
		}
		return item.Source, mount.Mount{Type: mount.TypeBind, Source: item.Source, Target: "/src", ReadOnly: true}, false, nil
	default:
		return "", mount.Mount{}, true, nil
	}
}

func outsiderVolumes(ctx context.Context, engine Engine, affected map[string]bool) (map[string]bool, error) {
	containers, err := engine.Containers(ctx)
	if err != nil {
		return nil, err
	}
	outsiders := map[string]bool{}
	for _, container := range containers {
		if affected[container.ID] || container.State != "running" {
			continue
		}
		spec, err := engine.Inspect(ctx, container.ID)
		if err != nil {
			continue
		}
		for _, item := range specMounts(spec) {
			if item.Type == mount.TypeVolume {
				outsiders[item.Source] = true
			}
		}
	}
	return outsiders, nil
}

func specMounts(spec engine.Spec) []mount.Mount {
	mounts := make([]mount.Mount, 0, len(spec.Mounts))
	for _, point := range spec.Mounts {
		item := mount.Mount{Type: point.Type, Source: point.Source, Target: point.Destination, ReadOnly: !point.RW}
		if point.Type == mount.TypeVolume {
			item.Source = point.Name
		}
		mounts = append(mounts, item)
	}
	return mounts
}
