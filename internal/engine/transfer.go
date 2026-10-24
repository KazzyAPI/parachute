package engine

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func (d *Docker) Wait(ctx context.Context, id string) (int64, error) {
	result := d.client.ContainerWait(ctx, id, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	select {
	case body, ok := <-result.Result:
		if !ok {
			return 0, writeError("wait for container", fmt.Errorf("wait result closed for %s", id))
		}
		return body.StatusCode, nil
	case err := <-result.Error:
		return 0, writeError("wait for container", err)
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func (d *Docker) Logs(ctx context.Context, id string, maxOutput int64) ([]byte, error) {
	logs, err := d.client.ContainerLogs(ctx, id, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return nil, writeError("read container logs", err)
	}
	defer func() { _ = logs.Close() }()
	output := &cappedBuffer{limit: maxOutput}
	if _, err := stdcopy.StdCopy(output, output, logs); err != nil {
		return nil, writeError("read container logs", err)
	}
	return output.Bytes(), nil
}

func (d *Docker) WriteFile(ctx context.Context, id, dir, name string, content []byte) error {
	var buf bytes.Buffer
	archive := tar.NewWriter(&buf)
	if err := archive.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
		return fmt.Errorf("build archive for %s: %w", name, err)
	}
	if _, err := archive.Write(content); err != nil {
		return fmt.Errorf("build archive for %s: %w", name, err)
	}
	if err := archive.Close(); err != nil {
		return fmt.Errorf("build archive for %s: %w", name, err)
	}
	_, err := d.client.CopyToContainer(ctx, id, client.CopyToContainerOptions{DestinationPath: dir, Content: &buf})
	return writeError("write file into container", err)
}

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

func (d *Docker) InspectVolume(ctx context.Context, name string) (Volume, bool, error) {
	volume, err := d.Volume(ctx, name)
	if cerrdefs.IsNotFound(err) {
		return Volume{}, false, nil
	}
	if err != nil {
		return Volume{}, false, err
	}
	return volume, true, nil
}
