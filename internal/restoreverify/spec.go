package restoreverify

import (
	"maps"
	"slices"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"

	"github.com/KazzyAPI/parachute/internal/engine"
)

const (
	helperLabel     = "parachute.helper"
	restoreLabel    = "parachute.restore-verify"
	composePrefix   = "com.docker.compose."
	parachutePrefix = "parachute."
	verifyPrefix    = "parachute.verify."
)

func SanitizeLabels(source map[string]string, runID string) map[string]string {
	labels := map[string]string{helperLabel: "true", restoreLabel: runID}
	for key, value := range source {
		if strings.HasPrefix(key, composePrefix) {
			continue
		}
		if strings.HasPrefix(key, parachutePrefix) && !strings.HasPrefix(key, verifyPrefix) {
			continue
		}
		labels[key] = value
	}
	return labels
}

func ThrowawaySpec(record engine.Spec, runID, suffix, networkName, serviceAlias string, volumeMounts []mount.Mount) engine.Spec {
	config := &container.Config{}
	if record.Config != nil {
		config.Image = record.Config.Image
		config.Cmd = slices.Clone(record.Config.Cmd)
		config.Entrypoint = slices.Clone(record.Config.Entrypoint)
		config.Env = slices.Clone(record.Config.Env)
		config.User = record.Config.User
		config.WorkingDir = record.Config.WorkingDir
		config.Healthcheck = cloneHealth(record.Config.Healthcheck)
		config.Labels = SanitizeLabels(record.Config.Labels, runID)
	}
	host := &container.HostConfig{
		Mounts:        volumeMounts,
		RestartPolicy: container.RestartPolicy{Name: "no"},
		Privileged:    false,
		CapDrop:       []string{"ALL"},
		SecurityOpt:   []string{"no-new-privileges"},
		NetworkMode:   container.NetworkMode(networkName),
	}
	endpoints := map[string]*network.EndpointSettings{
		networkName: {Aliases: []string{serviceAlias}},
	}
	return engine.Spec{
		Name:       ThrowawayContainerName(suffix, record.Name),
		Config:     config,
		HostConfig: host,
		Networks:   endpoints,
	}
}

func cloneHealth(check *container.HealthConfig) *container.HealthConfig {
	if check == nil {
		return nil
	}
	cloned := *check
	cloned.Test = slices.Clone(check.Test)
	return &cloned
}

func ServiceAlias(record engine.Spec) string {
	if record.Config != nil {
		if service := record.Config.Labels["com.docker.compose.service"]; service != "" {
			return service
		}
	}
	return record.Name
}

func ResourceLabels(runID string) map[string]string {
	return map[string]string{helperLabel: "true", restoreLabel: runID}
}

func OperatorLabels(spec engine.Spec) map[string]string {
	if spec.Config == nil {
		return map[string]string{}
	}
	return maps.Clone(spec.Config.Labels)
}

func ImageReference(spec engine.Spec) string {
	if spec.Config == nil {
		return ""
	}
	return spec.Config.Image
}
