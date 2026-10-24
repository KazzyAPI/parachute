package txn

import (
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"

	"github.com/KazzyAPI/parachute/internal/engine"
)

func anonymousVolumes(original engine.Spec) []container.MountPoint {
	bound := map[string]bool{}
	if original.HostConfig != nil {
		for _, bind := range original.HostConfig.Binds {
			parts := strings.SplitN(bind, ":", 3)
			if len(parts) >= 2 {
				bound[parts[1]] = true
			}
		}
		for _, m := range original.HostConfig.Mounts {
			bound[m.Target] = true
		}
	}
	var anonymous []container.MountPoint
	for _, m := range original.Mounts {
		if m.Type != mount.TypeVolume || bound[m.Destination] {
			continue
		}
		anonymous = append(anonymous, m)
	}
	return anonymous
}

func applyVolumeCarryOver(hostConfig *container.HostConfig, anonymous []container.MountPoint, removeVolumes bool) {
	if removeVolumes {
		return
	}
	for _, m := range anonymous {
		hostConfig.Binds = append(hostConfig.Binds, m.Name+":"+m.Destination)
	}
}
