package recreate

import (
	"strings"
	"testing"

	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
)

func TestApplyVolumeMountsKeepsOriginalAnonymousVolumeOverEmptyMount(t *testing.T) {
	name := strings.Repeat("ab", 32)
	host := &container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeVolume, Target: "/data"}}}
	original := engine.Spec{Mounts: []container.MountPoint{{Type: mount.TypeVolume, Name: name, Destination: "/data"}}}
	applyVolumeMounts(host, original, nil, nil, false)
	if len(host.Mounts) != 1 || host.Mounts[0].Source != name || host.Mounts[0].Target != "/data" {
		t.Fatalf("mounts %+v", host.Mounts)
	}
}
