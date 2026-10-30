package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

func (d *Docker) Pull(ctx context.Context, repository, digest string, auth RegistryAuth) error {
	ref := repository + "@" + digest
	response, err := d.client.ImagePull(ctx, ref, client.ImagePullOptions{RegistryAuth: string(auth)})
	if err != nil {
		return writeError("pull image", err)
	}
	if err := response.Wait(ctx); err != nil {
		return writeError("pull image", err)
	}
	return nil
}

func (d *Docker) Tag(ctx context.Context, source, target string) error {
	_, err := d.client.ImageTag(ctx, client.ImageTagOptions{Source: source, Target: target})
	return writeError("tag image", err)
}

func (d *Docker) Create(ctx context.Context, spec Spec) (string, error) {
	options := client.ContainerCreateOptions{
		Name:             spec.Name,
		Config:           spec.Config,
		HostConfig:       spec.HostConfig,
		NetworkingConfig: networkingConfig(spec.Networks),
	}
	created, err := d.client.ContainerCreate(ctx, options)
	if err != nil {
		return "", writeError("create container", err)
	}
	return created.ID, nil
}

func networkingConfig(endpoints map[string]*network.EndpointSettings) *network.NetworkingConfig {
	if len(endpoints) == 0 {
		return nil
	}
	return &network.NetworkingConfig{EndpointsConfig: endpoints}
}

func (d *Docker) Start(ctx context.Context, id string) error {
	_, err := d.client.ContainerStart(ctx, id, client.ContainerStartOptions{})
	return writeError("start container", err)
}

func (d *Docker) Stop(ctx context.Context, id, signal string, timeout time.Duration) error {
	seconds := int(timeout / time.Second)
	if seconds < 0 {
		seconds = 0
	}
	_, err := d.client.ContainerStop(ctx, id, client.ContainerStopOptions{Signal: signal, Timeout: &seconds})
	return writeError("stop container", err)
}

func (d *Docker) Rename(ctx context.Context, id, name string) error {
	_, err := d.client.ContainerRename(ctx, id, client.ContainerRenameOptions{NewName: name})
	return writeError("rename container", err)
}

func (d *Docker) Remove(ctx context.Context, id string, anonymousVolumes, force bool) error {
	_, err := d.client.ContainerRemove(ctx, id, client.ContainerRemoveOptions{Force: force, RemoveVolumes: anonymousVolumes})
	return writeError("remove container", err)
}

func (d *Docker) Exec(ctx context.Context, id string, cmd, env []string, limit time.Duration, maxOutput int64) (ExecResult, error) {
	attempt, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	fail := func(operation string, err error) (ExecResult, error) {
		if expired := attempt.Err(); expired != nil {
			err = expired
		}
		return ExecResult{}, writeError(operation, err)
	}
	created, err := d.client.ExecCreate(attempt, id, client.ExecCreateOptions{
		Cmd:          cmd,
		Env:          env,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return fail("create exec", err)
	}
	attached, err := d.client.ExecAttach(attempt, created.ID, client.ExecAttachOptions{})
	if err != nil {
		return fail("attach exec", err)
	}
	defer attached.Close()
	stop := context.AfterFunc(attempt, attached.Close)
	defer stop()
	output := &cappedBuffer{limit: maxOutput}
	if _, err := stdcopy.StdCopy(output, output, attached.Reader); err != nil {
		return fail("read exec output", err)
	}
	inspected, err := d.client.ExecInspect(attempt, created.ID, client.ExecInspectOptions{})
	if err != nil {
		return fail("inspect exec", err)
	}
	return ExecResult{ExitCode: inspected.ExitCode, Output: output.Bytes(), Truncated: output.truncated}, nil
}

func (d *Docker) Events(ctx context.Context, since time.Time, filters map[string][]string) (<-chan Event, <-chan error) {
	events := make(chan Event)
	errs := make(chan error, 1)
	clientFilters := client.Filters{}
	for key, values := range filters {
		for _, value := range values {
			clientFilters.Add(key, value)
		}
	}
	sinceText := ""
	if !since.IsZero() {
		sinceText = fmt.Sprintf("%d", since.Unix())
	}
	result := d.client.Events(ctx, client.EventsListOptions{Since: sinceText, Filters: clientFilters})
	go func() {
		defer close(events)
		defer close(errs)
		for {
			select {
			case message, ok := <-result.Messages:
				if !ok {
					return
				}
				event := Event{Type: string(message.Type), Action: string(message.Action), Time: time.Unix(0, message.TimeNano), ActorID: message.Actor.ID}
				select {
				case events <- event:
				case <-ctx.Done():
					errs <- ctx.Err()
					return
				}
			case err, ok := <-result.Err:
				if !ok {
					return
				}
				if err != nil && !errors.Is(err, context.Canceled) {
					errs <- err
				}
				return
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			}
		}
	}()
	return events, errs
}

func (d *Docker) RemoveImage(ctx context.Context, reference string) error {
	_, err := d.client.ImageRemove(ctx, reference, client.ImageRemoveOptions{})
	return writeError("remove image", err)
}

func (d *Docker) RemoveVolume(ctx context.Context, name string) error {
	_, err := d.client.VolumeRemove(ctx, name, client.VolumeRemoveOptions{Force: true})
	return writeError("remove volume", err)
}

type cappedBuffer struct {
	bytes.Buffer
	limit     int64
	truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	room := max(b.limit-int64(b.Len()), 0)
	if int64(len(p)) > room {
		b.truncated = true
	}
	b.Buffer.Write(p[:min(int64(len(p)), room)])
	return len(p), nil
}
