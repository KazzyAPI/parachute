package restoreverify

import (
	"context"
	"errors"
	"slices"

	"github.com/KazzyAPI/parachute/internal/backup"
	"github.com/KazzyAPI/parachute/internal/state"
)

func SelectTargets(document state.Document, options Options) []Target {
	type key struct {
		host  string
		stack string
	}
	groups := map[key][]state.SnapshotRecord{}
	for _, record := range document.Snapshots {
		if options.Host != "" && record.Host != options.Host {
			continue
		}
		if options.Stack != "" && record.Stack != options.Stack {
			continue
		}
		k := key{host: record.Host, stack: record.Stack}
		groups[k] = append(groups[k], record)
	}
	retention := options.Retention
	if retention < 0 {
		retention = 0
	}
	var targets []Target
	for k, records := range groups {
		if snapshot, ok := newestRetained(records, retention); ok {
			targets = append(targets, Target{Host: k.host, Stack: k.stack, Snapshot: snapshot})
		}
	}
	slices.SortFunc(targets, func(left, right Target) int {
		if left.Host != right.Host {
			if left.Host < right.Host {
				return -1
			}
			return 1
		}
		if left.Stack < right.Stack {
			return -1
		}
		if left.Stack > right.Stack {
			return 1
		}
		return 0
	})
	return targets
}

func newestRetained(records []state.SnapshotRecord, retention int) (state.SnapshotRecord, bool) {
	if len(records) == 0 {
		return state.SnapshotRecord{}, false
	}
	sorted := slices.Clone(records)
	slices.SortFunc(sorted, func(left, right state.SnapshotRecord) int {
		if left.Time.After(right.Time) {
			return -1
		}
		if left.Time.Before(right.Time) {
			return 1
		}
		return 0
	})
	if retention > 0 && len(sorted) > retention {
		sorted = sorted[:retention]
	}
	return sorted[0], true
}

func MountsForContainer(manifest backup.Manifest, container string) []backup.MountRecord {
	var mounts []backup.MountRecord
	for _, record := range manifest.Mounts {
		if record.Container == container {
			mounts = append(mounts, record)
		}
	}
	return mounts
}

func CleanupRuns(ctx context.Context, engine Engine, allowed []string) error {
	allowedSet := map[string]bool{}
	for _, id := range allowed {
		allowedSet[id] = true
	}
	return errors.Join(
		cleanupContainers(ctx, engine, allowedSet),
		cleanupVolumes(ctx, engine, allowedSet),
		cleanupNetworks(ctx, engine, allowedSet),
	)
}

func Removable(labels map[string]string, allowed map[string]bool) bool {
	if labels == nil || labels[helperLabel] != "true" {
		return false
	}
	runID := labels[restoreLabel]
	if runID == "" {
		return false
	}
	return allowed[runID]
}

func cleanupContainers(ctx context.Context, engine Engine, allowed map[string]bool) error {
	ids, err := engine.Helpers(ctx, map[string][]string{"label": {helperLabel + "=true"}})
	if err != nil {
		return err
	}
	var failures []error
	for _, id := range ids {
		spec, err := engine.Inspect(ctx, id)
		if err != nil {
			continue
		}
		if !Removable(OperatorLabels(spec), allowed) {
			continue
		}
		failures = append(failures, engine.Remove(ctx, id, true, true))
	}
	return errors.Join(failures...)
}

func cleanupVolumes(ctx context.Context, engine Engine, allowed map[string]bool) error {
	names, err := engine.ListVolumeNames(ctx, map[string][]string{"label": {helperLabel + "=true"}})
	if err != nil {
		return err
	}
	var failures []error
	for _, name := range names {
		labels, err := engine.VolumeLabels(ctx, name)
		if err != nil || !Removable(labels, allowed) {
			continue
		}
		failures = append(failures, engine.RemoveVolume(ctx, name))
	}
	return errors.Join(failures...)
}

func cleanupNetworks(ctx context.Context, engine Engine, allowed map[string]bool) error {
	names, err := engine.ListNetworkNames(ctx, map[string][]string{"label": {helperLabel + "=true"}})
	if err != nil {
		return err
	}
	var failures []error
	for _, name := range names {
		labels, err := engine.NetworkLabels(ctx, name)
		if err != nil || !Removable(labels, allowed) {
			continue
		}
		failures = append(failures, engine.RemoveNetwork(ctx, name))
	}
	return errors.Join(failures...)
}
