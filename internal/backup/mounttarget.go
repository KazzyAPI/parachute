package backup

import (
	"fmt"

	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/moby/moby/api/types/mount"
)

func mountForDestination(spec engine.Spec, destination string) (mount.Mount, error) {
	if spec.HostConfig == nil {
		return mount.Mount{}, fmt.Errorf("no mounts for %s", destination)
	}
	for _, item := range spec.HostConfig.Mounts {
		if item.Target != destination {
			continue
		}
		switch item.Type {
		case mount.TypeVolume, mount.TypeBind:
			return mount.Mount{Type: item.Type, Source: item.Source, Target: "/target"}, nil
		default:
			return mount.Mount{}, fmt.Errorf("unsupported mount type at %s", destination)
		}
	}
	return mount.Mount{}, fmt.Errorf("mount %s not found", destination)
}
