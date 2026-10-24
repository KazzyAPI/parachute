package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

type HelperSpec struct {
	Image       string
	Command     []string
	Mounts      []mount.Mount
	Labels      map[string]string
	CapAdd      []string
	NetworkMode string
}

func (d *Docker) RunHelper(ctx context.Context, spec HelperSpec) (string, error) {
	hostConfig := &container.HostConfig{
		Mounts:         spec.Mounts,
		ReadonlyRootfs: true,
		CapDrop:        []string{"ALL"},
		CapAdd:         spec.CapAdd,
		SecurityOpt:    []string{"no-new-privileges"},
		NetworkMode:    container.NetworkMode(spec.NetworkMode),
		AutoRemove:     false,
		RestartPolicy:  container.RestartPolicy{Name: "no"},
		Tmpfs:          map[string]string{"/tmp": "size=64m"},
	}
	if hostConfig.NetworkMode == "" {
		hostConfig.NetworkMode = "default"
	}
	config := &container.Config{
		Image:      spec.Image,
		Cmd:        spec.Command,
		Labels:     spec.Labels,
		Env:        []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
		WorkingDir: "/",
	}
	created, err := d.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: config, HostConfig: hostConfig, NetworkingConfig: nil,
	})
	if err != nil {
		return "", writeError("create helper", err)
	}
	if err := d.Start(ctx, created.ID); err != nil {
		_ = d.Remove(ctx, created.ID, false, true)
		return "", err
	}
	return created.ID, nil
}

func (d *Docker) Helpers(ctx context.Context, filters map[string][]string) ([]string, error) {
	clientFilters := client.Filters{}
	for key, values := range filters {
		for _, value := range values {
			clientFilters.Add(key, value)
		}
	}
	listed, err := d.client.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: clientFilters})
	if err != nil {
		return nil, writeError("list helpers", err)
	}
	ids := make([]string, 0, len(listed.Items))
	for _, item := range listed.Items {
		ids = append(ids, item.ID)
	}
	return ids, nil
}

func (d *Docker) RemoveHelpers(ctx context.Context, ids []string) error {
	var errs []error
	for _, id := range ids {
		errs = append(errs, d.Remove(ctx, id, false, true))
	}
	return errorsJoin(errs)
}

func errorsJoin(errs []error) error {
	var messages []string
	for _, err := range errs {
		if err != nil {
			messages = append(messages, err.Error())
		}
	}
	if len(messages) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(messages, "; "))
}
