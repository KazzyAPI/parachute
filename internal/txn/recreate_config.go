package txn

import (
	"maps"
	"reflect"
	"strings"
	"time"

	dockerspec "github.com/moby/docker-image-spec/specs-go/v1"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"

	"github.com/KazzyAPI/parachute/internal/engine"
)

const composeLabelPrefix = "com.docker.compose."

func buildReplacementSpec(original engine.Spec, newImage *dockerspec.DockerOCIImageConfig, imageRef string, plan Plan, now time.Time) engine.Spec {
	oldImage := original.ImageConfig
	config := cloneConfig(original.Config)
	config.Image = imageRef
	config.Env = mergeEnv(envMap(oldImage), envMap(newImage), config.Env)
	config.Labels = mergeLabels(labelMap(oldImage), labelMap(newImage), config.Labels)
	config.Labels["parachute.transaction"] = plan.ID
	config.Labels["parachute.updated-at"] = now.UTC().Format(time.RFC3339)
	config.Labels["parachute.previous-image"] = original.Config.Image
	config.Labels["parachute.snapshot"] = plan.Snapshot
	config.Cmd = mergeSlice(cmdOf(oldImage), cmdOf(newImage), config.Cmd)
	config.Entrypoint = mergeSlice(entrypointOf(oldImage), entrypointOf(newImage), config.Entrypoint)
	config.WorkingDir = mergeString(workingDirOf(oldImage), workingDirOf(newImage), config.WorkingDir)
	config.User = mergeString(userOf(oldImage), userOf(newImage), config.User)
	config.ExposedPorts = mergePorts(portsOf(oldImage), portsOf(newImage), config.ExposedPorts)
	config.Volumes = mergeVolumeSet(volumesOf(oldImage), volumesOf(newImage), config.Volumes)
	config.Healthcheck = mergeHealthcheck(healthcheckOf(oldImage), healthcheckOf(newImage), config.Healthcheck)
	hostConfig := cloneHostConfig(original.HostConfig)
	applyVolumeCarryOver(hostConfig, anonymousVolumes(original), plan.RemoveVolumes)
	return engine.Spec{Name: original.Name, Config: config, HostConfig: hostConfig, Networks: original.Networks}
}

func cloneConfig(original *container.Config) *container.Config {
	if original == nil {
		return &container.Config{}
	}
	clone := *original
	clone.Env = slicesClone(original.Env)
	clone.Cmd = slicesClone(original.Cmd)
	clone.Entrypoint = slicesClone(original.Entrypoint)
	clone.Labels = maps.Clone(original.Labels)
	if clone.Labels == nil {
		clone.Labels = map[string]string{}
	}
	clone.ExposedPorts = maps.Clone(original.ExposedPorts)
	clone.Volumes = maps.Clone(original.Volumes)
	return &clone
}

func cloneHostConfig(original *container.HostConfig) *container.HostConfig {
	if original == nil {
		return &container.HostConfig{}
	}
	clone := *original
	clone.Binds = slicesClone(original.Binds)
	clone.Mounts = slicesClone(original.Mounts)
	return &clone
}

func slicesClone[T any](s []T) []T {
	if s == nil {
		return nil
	}
	clone := make([]T, len(s))
	copy(clone, s)
	return clone
}

func envMap(image *dockerspec.DockerOCIImageConfig) map[string]string {
	m := map[string]string{}
	if image == nil {
		return m
	}
	for _, entry := range image.Env {
		if k, v, ok := strings.Cut(entry, "="); ok {
			m[k] = v
		}
	}
	return m
}

func mergeEnv(oldImage, newImage map[string]string, current []string) []string {
	currentMap := map[string]string{}
	order := make([]string, 0, len(current))
	for _, entry := range current {
		k, v, _ := strings.Cut(entry, "=")
		currentMap[k] = v
		order = append(order, k)
	}
	for k := range newImage {
		if _, seen := currentMap[k]; !seen {
			order = append(order, k)
		}
	}
	result := make([]string, 0, len(order))
	seen := map[string]bool{}
	for _, k := range order {
		if seen[k] {
			continue
		}
		seen[k] = true
		curV, inCur := currentMap[k]
		oldV, inOld := oldImage[k]
		newV, inNew := newImage[k]
		overridden := inCur && (!inOld || curV != oldV)
		switch {
		case overridden:
			result = append(result, k+"="+curV)
		case inNew:
			result = append(result, k+"="+newV)
		case inCur:
			result = append(result, k+"="+curV)
		}
	}
	return result
}

func labelMap(image *dockerspec.DockerOCIImageConfig) map[string]string {
	if image == nil {
		return map[string]string{}
	}
	return image.Labels
}

func mergeLabels(oldImage, newImage, current map[string]string) map[string]string {
	result := map[string]string{}
	for k, v := range current {
		if strings.HasPrefix(k, composeLabelPrefix) {
			result[k] = v
			continue
		}
		oldV, inOld := oldImage[k]
		if !inOld || v != oldV {
			result[k] = v
		}
	}
	for k, v := range newImage {
		if _, kept := result[k]; !kept {
			result[k] = v
		}
	}
	for k, v := range current {
		if strings.HasPrefix(k, composeLabelPrefix) {
			result[k] = v
		}
	}
	return result
}

func cmdOf(image *dockerspec.DockerOCIImageConfig) []string {
	if image == nil {
		return nil
	}
	return image.Cmd
}

func entrypointOf(image *dockerspec.DockerOCIImageConfig) []string {
	if image == nil {
		return nil
	}
	return image.Entrypoint
}

func mergeSlice(oldImage, newImage, current []string) []string {
	if !slicesEqual(current, oldImage) {
		return current
	}
	return newImage
}

func slicesEqual(a, b []string) bool {
	return reflect.DeepEqual(a, b)
}

func workingDirOf(image *dockerspec.DockerOCIImageConfig) string {
	if image == nil {
		return ""
	}
	return image.WorkingDir
}

func userOf(image *dockerspec.DockerOCIImageConfig) string {
	if image == nil {
		return ""
	}
	return image.User
}

func mergeString(oldImage, newImage, current string) string {
	if current != oldImage {
		return current
	}
	return newImage
}

func portsOf(image *dockerspec.DockerOCIImageConfig) network.PortSet {
	set := network.PortSet{}
	if image == nil {
		return set
	}
	for raw := range image.ExposedPorts {
		if port, err := network.ParsePort(raw); err == nil {
			set[port] = struct{}{}
		}
	}
	return set
}

func mergePorts(oldImage, newImage, current network.PortSet) network.PortSet {
	if !maps.Equal(current, oldImage) {
		return current
	}
	return newImage
}

func volumesOf(image *dockerspec.DockerOCIImageConfig) map[string]struct{} {
	if image == nil {
		return map[string]struct{}{}
	}
	return image.Volumes
}

func mergeVolumeSet(oldImage, newImage, current map[string]struct{}) map[string]struct{} {
	if !maps.Equal(current, oldImage) {
		return current
	}
	return newImage
}

func healthcheckOf(image *dockerspec.DockerOCIImageConfig) *container.HealthConfig {
	if image == nil {
		return nil
	}
	return image.Healthcheck
}

func mergeHealthcheck(oldImage, newImage, current *container.HealthConfig) *container.HealthConfig {
	if !reflect.DeepEqual(current, oldImage) {
		return current
	}
	return newImage
}
