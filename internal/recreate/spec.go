package recreate

import (
	"context"
	"maps"
	"slices"
	"strings"
	"time"

	dockerspec "github.com/moby/docker-image-spec/specs-go/v1"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"

	"github.com/KazzyAPI/parachute/internal/engine"
)

const swarmServiceLabel = "com.docker.swarm.service.id"

type Settings struct {
	RemoveVolumes bool
	CommittedAt   time.Time
}

func checkReproducible(ctx context.Context, e Engine, spec engine.Spec) (string, bool) {
	if spec.HostConfig != nil && spec.HostConfig.AutoRemove {
		return "AutoRemove", false
	}
	if spec.Config != nil && spec.Config.Labels[swarmServiceLabel] != "" {
		return swarmServiceLabel, false
	}
	if spec.HostConfig == nil || !strings.HasPrefix(string(spec.HostConfig.NetworkMode), "container:") {
		return "", true
	}
	peer := strings.TrimPrefix(string(spec.HostConfig.NetworkMode), "container:")
	if _, err := e.Inspect(ctx, peer); err != nil {
		return "NetworkMode", false
	}
	return "", true
}

func buildReplacement(original engine.Spec, oldImage, newImage *dockerspec.DockerOCIImageConfig, planID, snapshot, previousImage string, settings Settings) engine.Spec {
	config := cloneConfig(original.Config)
	host := cloneHostConfig(original.HostConfig)
	networks := cloneNetworks(original.Networks)
	applyImageDefaults(config, host, original, oldImage, newImage, settings.RemoveVolumes)
	applyBookkeeping(config, planID, snapshot, previousImage, settings.CommittedAt)
	return engine.Spec{Name: original.Name, Config: config, HostConfig: host, Networks: networks}
}

func cloneConfig(config *container.Config) *container.Config {
	if config == nil {
		return &container.Config{}
	}
	cloned := *config
	cloned.ExposedPorts = maps.Clone(config.ExposedPorts)
	cloned.Env = slices.Clone(config.Env)
	cloned.Cmd = slices.Clone(config.Cmd)
	cloned.Entrypoint = slices.Clone(config.Entrypoint)
	cloned.Labels = maps.Clone(config.Labels)
	cloned.Volumes = maps.Clone(config.Volumes)
	cloned.Healthcheck = cloneHealthcheck(config.Healthcheck)
	return &cloned
}

func cloneHostConfig(host *container.HostConfig) *container.HostConfig {
	if host == nil {
		return &container.HostConfig{}
	}
	cloned := *host
	cloned.PortBindings = maps.Clone(host.PortBindings)
	cloned.Binds = slices.Clone(host.Binds)
	cloned.Mounts = slices.Clone(host.Mounts)
	cloned.Tmpfs = maps.Clone(host.Tmpfs)
	cloned.Ulimits = slices.Clone(host.Ulimits)
	cloned.Devices = slices.Clone(host.Devices)
	cloned.CapAdd = slices.Clone(host.CapAdd)
	cloned.CapDrop = slices.Clone(host.CapDrop)
	cloned.DNS = slices.Clone(host.DNS)
	cloned.DNSSearch = slices.Clone(host.DNSSearch)
	cloned.ExtraHosts = slices.Clone(host.ExtraHosts)
	cloned.GroupAdd = slices.Clone(host.GroupAdd)
	return &cloned
}

func cloneNetworks(networks map[string]*network.EndpointSettings) map[string]*network.EndpointSettings {
	if len(networks) == 0 {
		return nil
	}
	cloned := make(map[string]*network.EndpointSettings, len(networks))
	for key, value := range networks {
		if value == nil {
			continue
		}
		endpoint := *value
		endpoint.Aliases = slices.Clone(value.Aliases)
		cloned[key] = &endpoint
	}
	return cloned
}

func cloneHealthcheck(check *container.HealthConfig) *container.HealthConfig {
	if check == nil {
		return nil
	}
	cloned := *check
	cloned.Test = slices.Clone(check.Test)
	return &cloned
}

func applyImageDefaults(config *container.Config, host *container.HostConfig, original engine.Spec, oldImage, newImage *dockerspec.DockerOCIImageConfig, removeVolumes bool) {
	if newImage == nil {
		return
	}
	originalConfig := original.Config
	if originalConfig == nil {
		originalConfig = &container.Config{}
	}
	if newImage.User != "" && inheritedString(originalConfig.User, oldImage, func(image *dockerspec.DockerOCIImageConfig) string { return image.User }) {
		config.User = newImage.User
	}
	if newImage.WorkingDir != "" && inheritedString(originalConfig.WorkingDir, oldImage, func(image *dockerspec.DockerOCIImageConfig) string { return image.WorkingDir }) {
		config.WorkingDir = newImage.WorkingDir
	}
	if inheritedSlice(originalConfig.Cmd, oldImage, func(image *dockerspec.DockerOCIImageConfig) []string { return image.Cmd }) {
		config.Cmd = slices.Clone(newImage.Cmd)
	}
	if inheritedSlice(originalConfig.Entrypoint, oldImage, func(image *dockerspec.DockerOCIImageConfig) []string { return image.Entrypoint }) {
		config.Entrypoint = slices.Clone(newImage.Entrypoint)
	}
	config.Env = mergeEnv(originalConfig.Env, oldImage, newImage)
	config.Labels = mergeLabels(originalConfig.Labels, oldImage, newImage)
	if inheritedPorts(originalConfig.ExposedPorts, oldImage) {
		config.ExposedPorts = imagePortSet(newImage)
	}
	if inheritedHealthcheck(originalConfig.Healthcheck, oldImage) {
		config.Healthcheck = cloneHealthcheck(newImage.Healthcheck)
	}
	applyVolumeMounts(host, original, oldImage, newImage, removeVolumes)
}

func inheritedString(value string, image *dockerspec.DockerOCIImageConfig, fromImage func(*dockerspec.DockerOCIImageConfig) string) bool {
	if value == "" {
		return true
	}
	return image != nil && value == fromImage(image)
}

func inheritedSlice(value []string, image *dockerspec.DockerOCIImageConfig, fromImage func(*dockerspec.DockerOCIImageConfig) []string) bool {
	if len(value) == 0 {
		return true
	}
	return image != nil && slices.Equal(value, fromImage(image))
}

func inheritedPorts(ports network.PortSet, image *dockerspec.DockerOCIImageConfig) bool {
	if len(ports) == 0 {
		return true
	}
	return image != nil && maps.Equal(ports, imagePortSet(image))
}

func imagePortSet(image *dockerspec.DockerOCIImageConfig) network.PortSet {
	if image == nil || len(image.ExposedPorts) == 0 {
		return nil
	}
	ports := network.PortSet{}
	for key := range image.ExposedPorts {
		ports[network.MustParsePort(key)] = struct{}{}
	}
	return ports
}

func inheritedHealthcheck(check *container.HealthConfig, image *dockerspec.DockerOCIImageConfig) bool {
	if check == nil {
		return true
	}
	return image != nil && healthEqual(check, image.Healthcheck)
}

func healthEqual(left, right *container.HealthConfig) bool {
	if left == nil || right == nil {
		return left == right
	}
	return slices.Equal(left.Test, right.Test) && left.Interval == right.Interval && left.Timeout == right.Timeout &&
		left.Retries == right.Retries && left.StartPeriod == right.StartPeriod && left.StartInterval == right.StartInterval
}

func mergeEnv(original []string, oldImage, newImage *dockerspec.DockerOCIImageConfig) []string {
	originalValues := envMap(original)
	oldValues := envMap(imageEnv(oldImage))
	explicit := map[string]string{}
	for key, value := range originalValues {
		if old, ok := oldValues[key]; !ok || old != value {
			explicit[key] = value
		}
	}
	merged := envMap(imageEnv(newImage))
	maps.Copy(merged, explicit)
	return envList(merged)
}

func mergeLabels(original map[string]string, oldImage, newImage *dockerspec.DockerOCIImageConfig) map[string]string {
	labels := map[string]string{}
	if newImage != nil {
		maps.Copy(labels, newImage.Labels)
	}
	var oldLabels map[string]string
	if oldImage != nil {
		oldLabels = oldImage.Labels
	}
	for key, value := range original {
		if strings.HasPrefix(key, "com.docker.compose.") {
			labels[key] = value
			continue
		}
		if old, ok := oldLabels[key]; !ok || old != value {
			labels[key] = value
		}
	}
	return labels
}

func imageEnv(image *dockerspec.DockerOCIImageConfig) []string {
	if image == nil {
		return nil
	}
	return image.Env
}

func envMap(items []string) map[string]string {
	values := map[string]string{}
	for _, item := range items {
		key, value, _ := strings.Cut(item, "=")
		values[key] = value
	}
	return values
}

func envList(values map[string]string) []string {
	keys := slices.Sorted(maps.Keys(values))
	list := make([]string, 0, len(keys))
	for _, key := range keys {
		list = append(list, key+"="+values[key])
	}
	return list
}

func applyVolumeMounts(host *container.HostConfig, original engine.Spec, oldImage, newImage *dockerspec.DockerOCIImageConfig, removeVolumes bool) {
	if host == nil {
		return
	}
	if removeVolumes {
		host.Mounts = withoutAnonymous(host.Mounts)
		for path := range newImageDeclared(newImage) {
			host.Mounts = append(host.Mounts, mount.Mount{Type: mount.TypeVolume, Target: path})
		}
		return
	}
	kept := map[string]mount.Mount{}
	for _, item := range host.Mounts {
		if item.Type == mount.TypeVolume && item.Source == "" {
			kept[item.Target] = item
		}
	}
	for _, item := range original.Mounts {
		if item.Type != mount.TypeVolume || item.Name != "" {
			continue
		}
		if _, ok := kept[item.Destination]; ok {
			continue
		}
		kept[item.Destination] = mount.Mount{Type: mount.TypeVolume, Source: item.Name, Target: item.Destination}
	}
	host.Mounts = withoutAnonymous(host.Mounts)
	for _, item := range kept {
		host.Mounts = append(host.Mounts, item)
	}
	for path := range newImageDeclared(newImage) {
		if _, ok := kept[path]; ok {
			continue
		}
		if declaredBy(oldImage, path) {
			continue
		}
		host.Mounts = append(host.Mounts, mount.Mount{Type: mount.TypeVolume, Target: path})
	}
}

func withoutAnonymous(mounts []mount.Mount) []mount.Mount {
	filtered := make([]mount.Mount, 0, len(mounts))
	for _, item := range mounts {
		if item.Type == mount.TypeVolume && item.Source == "" {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func newImageDeclared(image *dockerspec.DockerOCIImageConfig) map[string]struct{} {
	declared := map[string]struct{}{}
	if image == nil {
		return declared
	}
	for path := range image.Volumes {
		declared[path] = struct{}{}
	}
	return declared
}

func declaredBy(image *dockerspec.DockerOCIImageConfig, path string) bool {
	if image == nil {
		return false
	}
	_, ok := image.Volumes[path]
	return ok
}

func applyBookkeeping(config *container.Config, planID, snapshot, previousImage string, committed time.Time) {
	if config.Labels == nil {
		config.Labels = map[string]string{}
	}
	config.Labels["parachute.transaction"] = planID
	config.Labels["parachute.updated-at"] = committed.UTC().Format(time.RFC3339)
	config.Labels["parachute.previous-image"] = previousImage
	if snapshot == "" {
		snapshot = "none"
	}
	config.Labels["parachute.snapshot"] = snapshot
}

func configsMatch(expected, created engine.Spec) (string, bool) {
	if expected.Config == nil || created.Config == nil {
		return "Config", false
	}
	if expected.HostConfig == nil || created.HostConfig == nil {
		return "HostConfig", false
	}
	if expected.Config.Hostname != created.Config.Hostname {
		return "Hostname", false
	}
	if expected.Config.User != created.Config.User {
		return "User", false
	}
	if expected.Config.WorkingDir != created.Config.WorkingDir {
		return "WorkingDir", false
	}
	if !slices.Equal(expected.Config.Cmd, created.Config.Cmd) {
		return "Cmd", false
	}
	if !slices.Equal(expected.Config.Entrypoint, created.Config.Entrypoint) {
		return "Entrypoint", false
	}
	if !envEqualIgnoringBookkeeping(expected.Config.Env, created.Config.Env) {
		return "Env", false
	}
	if !labelsEqualIgnoringBookkeeping(expected.Config.Labels, created.Config.Labels) {
		return "Labels", false
	}
	if !maps.Equal(expected.Config.ExposedPorts, created.Config.ExposedPorts) {
		return "ExposedPorts", false
	}
	if !healthEqual(expected.Config.Healthcheck, created.Config.Healthcheck) {
		return "Healthcheck", false
	}
	if expected.HostConfig.RestartPolicy.Name != created.HostConfig.RestartPolicy.Name ||
		expected.HostConfig.RestartPolicy.MaximumRetryCount != created.HostConfig.RestartPolicy.MaximumRetryCount {
		return "RestartPolicy", false
	}
	if expected.HostConfig.Memory != created.HostConfig.Memory {
		return "Memory", false
	}
	if !portBindingsEqual(expected.HostConfig.PortBindings, created.HostConfig.PortBindings) {
		return "PortBindings", false
	}
	if !slices.Equal(expected.HostConfig.Binds, created.HostConfig.Binds) {
		return "Binds", false
	}
	if !mountsEqual(expected.HostConfig.Mounts, created.HostConfig.Mounts) {
		return "Mounts", false
	}
	if expected.HostConfig.NetworkMode != created.HostConfig.NetworkMode {
		return "NetworkMode", false
	}
	return "", true
}

func envEqualIgnoringBookkeeping(left, right []string) bool {
	return maps.Equal(envMap(left), envMap(right))
}

func labelsEqualIgnoringBookkeeping(left, right map[string]string) bool {
	return maps.Equal(withoutParachuteLabels(left), withoutParachuteLabels(right))
}

func withoutParachuteLabels(labels map[string]string) map[string]string {
	filtered := map[string]string{}
	for key, value := range labels {
		if !strings.HasPrefix(key, "parachute.") {
			filtered[key] = value
		}
	}
	return filtered
}

func portBindingsEqual(left, right map[network.Port][]network.PortBinding) bool {
	if len(left) != len(right) {
		return false
	}
	for key, leftBindings := range left {
		rightBindings, ok := right[key]
		if !ok || len(leftBindings) != len(rightBindings) {
			return false
		}
		for index := range leftBindings {
			if leftBindings[index] != rightBindings[index] {
				return false
			}
		}
	}
	return true
}

func mountsEqual(left, right []mount.Mount) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Type != right[index].Type || left[index].Source != right[index].Source || left[index].Target != right[index].Target {
			return false
		}
	}
	return true
}
