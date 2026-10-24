package backup

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/moby/moby/api/types/mount"
)

func hostPathForDestination(ctx context.Context, engine Engine, spec engine.Spec, destination string) (string, error) {
	if spec.HostConfig == nil {
		return "", fmt.Errorf("no mounts for %s", destination)
	}
	for _, item := range spec.HostConfig.Mounts {
		if item.Target != destination {
			continue
		}
		switch item.Type {
		case mount.TypeVolume:
			volume, err := engine.Volume(ctx, item.Source)
			if err != nil {
				return "", err
			}
			return volume.Mountpoint, nil
		case mount.TypeBind:
			return resolveOrSelf(item.Source), nil
		default:
			return "", fmt.Errorf("unsupported mount type at %s", destination)
		}
	}
	return "", fmt.Errorf("mount %s not found", destination)
}

func resolveOrSelf(source string) string {
	if resolved, err := filepath.EvalSymlinks(source); err == nil {
		return resolved
	}
	return source
}
