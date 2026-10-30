package engine

import (
	"context"
	"fmt"
	"strings"

	cerrdefs "github.com/containerd/errdefs"
	dockerspec "github.com/moby/docker-image-spec/specs-go/v1"
	"github.com/moby/moby/client"
)

func (d *Docker) Inspect(ctx context.Context, id string) (Spec, error) {
	inspected, err := d.client.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return Spec{}, fmt.Errorf("inspect container %s: %w", id, err)
	}
	details := inspected.Container
	spec := Spec{
		ID:      details.ID,
		Name:    strings.TrimPrefix(details.Name, "/"),
		ImageID: details.Image,
		Mounts:  details.Mounts,
	}
	if details.Config != nil {
		spec.Config = details.Config
	}
	if details.HostConfig != nil {
		spec.HostConfig = details.HostConfig
	}
	if details.NetworkSettings != nil {
		spec.Networks = details.NetworkSettings.Networks
	}
	spec.Mounts = details.Mounts
	if details.State != nil && details.State.Health != nil {
		spec.HealthStatus = string(details.State.Health.Status)
	}
	if details.Config != nil {
		for _, reference := range []string{details.Image, details.Config.Image} {
			if reference == "" {
				continue
			}
			image, err := d.client.ImageInspect(ctx, reference)
			if cerrdefs.IsNotFound(err) {
				continue
			}
			if err != nil {
				return Spec{}, fmt.Errorf("inspect image %s: %w", reference, err)
			}
			spec.ImageConfig = image.Config
			break
		}
	}
	return spec, nil
}

func (d *Docker) ImageConfig(ctx context.Context, reference string) (*dockerspec.DockerOCIImageConfig, error) {
	inspected, err := d.client.ImageInspect(ctx, reference)
	if err != nil {
		return nil, fmt.Errorf("inspect image %s: %w", reference, err)
	}
	return inspected.Config, nil
}
