package engine

import (
	"context"
	"fmt"
	"strings"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type Image struct {
	ID      string
	Digests []string
	Labels  map[string]string
}

type Docker struct {
	client *client.Client
}

func NewDocker(endpoint Endpoint) (*Docker, error) {
	options, err := endpoint.clientOptions()
	if err != nil {
		return nil, err
	}
	dockerClient, err := client.New(options...)
	if err != nil {
		return nil, fmt.Errorf("cannot create Docker client for %s: %w", endpoint.Host, err)
	}
	return &Docker{client: dockerClient}, nil
}

func (d *Docker) Verify(ctx context.Context) error {
	if _, err := d.client.ServerVersion(ctx, client.ServerVersionOptions{}); err != nil {
		return fmt.Errorf("cannot reach Docker engine: %w", err)
	}
	return nil
}

func (d *Docker) Containers(ctx context.Context) ([]domain.Container, error) {
	listed, err := d.client.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	containers := make([]domain.Container, 0, len(listed.Items))
	for _, summary := range listed.Items {
		inspected, err := d.client.ContainerInspect(ctx, summary.ID, client.ContainerInspectOptions{})
		if cerrdefs.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect container %s: %w", summary.ID, err)
		}
		details := inspected.Container
		container := domain.Container{
			ID:      details.ID,
			Name:    strings.TrimPrefix(details.Name, "/"),
			ImageID: details.Image,
		}
		if details.State != nil {
			container.State = string(details.State.Status)
		}
		if details.Config != nil {
			container.Labels = details.Config.Labels
			container.ImageRef = details.Config.Image
			container.Env = details.Config.Env
		}
		containers = append(containers, container)
	}
	return containers, nil
}

func (d *Docker) Image(ctx context.Context, reference string) (Image, bool, error) {
	inspected, err := d.client.ImageInspect(ctx, reference)
	if cerrdefs.IsNotFound(err) {
		return Image{}, false, nil
	}
	if err != nil {
		return Image{}, false, fmt.Errorf("inspect image %s: %w", reference, err)
	}
	image := Image{ID: inspected.ID}
	if inspected.Config != nil {
		image.Labels = inspected.Config.Labels
	}
	for _, repoDigest := range inspected.RepoDigests {
		if _, digest, found := strings.Cut(repoDigest, "@"); found {
			image.Digests = append(image.Digests, digest)
		}
	}
	return image, true, nil
}

func (d *Docker) Close() error {
	return d.client.Close()
}
