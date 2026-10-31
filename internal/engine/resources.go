package engine

import (
	"context"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

func (d *Docker) CreateInternalNetwork(ctx context.Context, name string, labels map[string]string) error {
	_, err := d.client.NetworkCreate(ctx, name, client.NetworkCreateOptions{
		Driver:   "bridge",
		Labels:   labels,
		Internal: true,
		IPAM:     &network.IPAM{Driver: "default"},
	})
	if err != nil {
		return writeError("create network", err)
	}
	return nil
}

func (d *Docker) RemoveNetwork(ctx context.Context, name string) error {
	_, err := d.client.NetworkRemove(ctx, name, client.NetworkRemoveOptions{})
	return writeError("remove network", err)
}

func (d *Docker) CreateLabeledVolume(ctx context.Context, name string, labels map[string]string) error {
	_, err := d.client.VolumeCreate(ctx, client.VolumeCreateOptions{Name: name, Labels: labels})
	return writeError("create volume", err)
}

func (d *Docker) ListVolumeNames(ctx context.Context, filters map[string][]string) ([]string, error) {
	clientFilters := client.Filters{}
	for key, values := range filters {
		for _, value := range values {
			clientFilters.Add(key, value)
		}
	}
	listed, err := d.client.VolumeList(ctx, client.VolumeListOptions{Filters: clientFilters})
	if err != nil {
		return nil, writeError("list volumes", err)
	}
	names := make([]string, 0, len(listed.Items))
	for _, item := range listed.Items {
		names = append(names, item.Name)
	}
	return names, nil
}

func (d *Docker) ListNetworkNames(ctx context.Context, filters map[string][]string) ([]string, error) {
	clientFilters := client.Filters{}
	for key, values := range filters {
		for _, value := range values {
			clientFilters.Add(key, value)
		}
	}
	listed, err := d.client.NetworkList(ctx, client.NetworkListOptions{Filters: clientFilters})
	if err != nil {
		return nil, writeError("list networks", err)
	}
	names := make([]string, 0, len(listed.Items))
	for _, item := range listed.Items {
		names = append(names, item.Name)
	}
	return names, nil
}

func (d *Docker) EnsureImage(ctx context.Context, reference string) error {
	_, found, err := d.Image(ctx, reference)
	if err != nil {
		return err
	}
	if found {
		return nil
	}
	response, err := d.client.ImagePull(ctx, reference, client.ImagePullOptions{})
	if err != nil {
		return writeError("pull image", err)
	}
	if err := response.Wait(ctx); err != nil {
		return writeError("pull image", err)
	}
	return nil
}

func (d *Docker) NetworkLabels(ctx context.Context, name string) (map[string]string, error) {
	inspected, err := d.client.NetworkInspect(ctx, name, client.NetworkInspectOptions{})
	if err != nil {
		return nil, writeError("inspect network", err)
	}
	return inspected.Network.Labels, nil
}

func (d *Docker) NetworkExists(ctx context.Context, name string) (bool, error) {
	_, err := d.client.NetworkInspect(ctx, name, client.NetworkInspectOptions{})
	if cerrdefs.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, writeError("inspect network", err)
	}
	return true, nil
}
