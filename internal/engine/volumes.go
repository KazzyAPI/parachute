package engine

import (
	"context"
	"fmt"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
)

type Volume struct {
	Name       string
	Driver     string
	Mountpoint string
	Options    map[string]string
}

func (d *Docker) Volume(ctx context.Context, name string) (Volume, error) {
	inspected, err := d.client.VolumeInspect(ctx, name, client.VolumeInspectOptions{})
	if err != nil {
		return Volume{}, writeError("inspect volume", err)
	}
	volume := inspected.Volume
	return Volume{Name: volume.Name, Driver: volume.Driver, Mountpoint: volume.Mountpoint, Options: volume.Options}, nil
}

func (d *Docker) EnsureVolume(ctx context.Context, name string) error {
	_, err := d.client.VolumeInspect(ctx, name, client.VolumeInspectOptions{})
	if err == nil {
		return nil
	}
	if !cerrdefs.IsNotFound(err) {
		return writeError("inspect volume", err)
	}
	_, err = d.client.VolumeCreate(ctx, client.VolumeCreateOptions{Name: name})
	return writeError("create volume", err)
}

func (d *Docker) CopyToContainer(ctx context.Context, id, path string, data []byte) error {
	archive, err := tarBytes(data, path)
	if err != nil {
		return err
	}
	_, err = d.client.CopyToContainer(ctx, id, client.CopyToContainerOptions{DestinationPath: "/", Content: archive})
	return writeError("copy to container", err)
}

func (d *Docker) CopyFromContainer(ctx context.Context, id, path string) ([]byte, error) {
	result, err := d.client.CopyFromContainer(ctx, id, client.CopyFromContainerOptions{SourcePath: path})
	if err != nil {
		return nil, writeError("copy from container", err)
	}
	defer result.Content.Close()
	return readTarFile(result.Content, pathBase(path))
}

func pathBase(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

func (d *Docker) Wait(ctx context.Context, id string) (int, error) {
	result := d.client.ContainerWait(ctx, id, client.ContainerWaitOptions{Condition: "not-running"})
	select {
	case <-ctx.Done():
		return -1, ctx.Err()
	case err := <-result.Error:
		if err != nil {
			return -1, writeError("wait container", err)
		}
	case status := <-result.Result:
		return int(status.StatusCode), nil
	}
	return -1, fmt.Errorf("wait container %s: no status", id)
}
