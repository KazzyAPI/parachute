package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/moby/moby/api/types/mount"
)

func planMounts(ctx context.Context, engine Engine, hostFS HostFS, labels LabelReader, storePath string, container domain.Container, spec engine.Spec, outsiders map[string]bool) ([]plannedMount, []SkippedRecord, error) {
	excludes := excludeSet(labels, container)
	var planned []plannedMount
	var skipped []SkippedRecord
	if spec.HostConfig == nil {
		return planned, skipped, nil
	}
	for _, item := range spec.HostConfig.Mounts {
		if item.ReadOnly || item.Type == mount.TypeTmpfs {
			continue
		}
		if excludes[item.Target] {
			continue
		}
		source, skip, err := mountSource(ctx, engine, hostFS, item)
		if err != nil {
			return nil, nil, err
		}
		if skip {
			skipped = append(skipped, SkippedRecord{Container: container.Name, Destination: item.Target, Source: source})
			continue
		}
		resolved := source
		if hostFS != nil {
			resolved, err = hostFS.Resolve(ctx, source)
			if err != nil {
				resolved = source
			}
		}
		if systemMount(resolved, storePath) {
			skipped = append(skipped, SkippedRecord{Container: container.Name, Destination: item.Target, Source: resolved})
			continue
		}
		consistency := "stopped"
		if outsiders[item.Source] {
			consistency = "live"
		}
		planned = append(planned, plannedMount{
			Container: container.Name, Destination: item.Target, Source: source, Kind: item.Type,
			VolumeName: item.Source, Consistency: consistency, HostSource: resolved,
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

func mountSource(ctx context.Context, engine Engine, hostFS HostFS, item mount.Mount) (string, bool, error) {
	switch item.Type {
	case mount.TypeVolume:
		volume, err := engine.Volume(ctx, item.Source)
		if err != nil {
			return "", false, err
		}
		device := volume.Options["device"]
		if device != "" {
			resolved := device
			if hostFS != nil {
				resolved, _ = hostFS.Resolve(ctx, device)
			}
			if systemMount(resolved, "") {
				return resolved, true, nil
			}
		}
		return volume.Mountpoint, false, nil
	case mount.TypeBind:
		info, err := filepath.EvalSymlinks(item.Source)
		if err != nil {
			return item.Source, true, nil
		}
		stat, err := os.Stat(info)
		if err != nil || !(stat.IsDir() || stat.Mode().IsRegular()) {
			return item.Source, true, nil
		}
		return item.Source, false, nil
	default:
		return "", true, nil
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
		if err != nil || spec.HostConfig == nil {
			continue
		}
		for _, item := range spec.HostConfig.Mounts {
			if item.Type == mount.TypeVolume {
				outsiders[item.Source] = true
			}
		}
	}
	return outsiders, nil
}
