package txn

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/containerd/errdefs"

	"github.com/KazzyAPI/parachute/internal/engine"
)

const swarmServiceLabel = "com.docker.swarm.service.id"
const defaultStopTimeout = 10 * time.Second

type DockerRecreator struct {
	engine RecreateEngine
	now    func() time.Time
}

func NewDockerRecreator(engine RecreateEngine, now func() time.Time) *DockerRecreator {
	return &DockerRecreator{engine: engine, now: now}
}

func (r *DockerRecreator) Replacement(ctx context.Context, plan Plan, container string) (string, error) {
	target, ok := findTarget(plan, container)
	if !ok {
		return "", fmt.Errorf("transaction %s has no target for container %s", plan.ID, container)
	}
	original, err := r.engine.Inspect(ctx, container)
	if err != nil {
		return "", fmt.Errorf("inspect original container: %w", err)
	}
	if err := r.checkReproducible(ctx, original); err != nil {
		return "", err
	}
	approvedRef := target.Image + "@" + target.Digest
	if err := r.engine.Pull(ctx, target.Image, target.Digest, ""); err != nil {
		return "", &PullFailedError{Repository: target.Image, Approved: target.Digest, Detail: err.Error()}
	}
	pulled, found, err := r.engine.Image(ctx, approvedRef)
	if err != nil {
		return "", &PullFailedError{Repository: target.Image, Approved: target.Digest, Detail: err.Error()}
	}
	if !found || !slices.Contains(pulled.Digests, target.Digest) {
		return "", &PullFailedError{Repository: target.Image, Approved: target.Digest, Detail: strings.Join(pulled.Digests, ",")}
	}
	originalRef := original.Config.Image
	if err := r.engine.Tag(ctx, approvedRef, originalRef); err != nil {
		return "", fmt.Errorf("retag %s as %s: %w", approvedRef, originalRef, err)
	}
	newImage, err := r.engine.ImageConfig(ctx, approvedRef)
	if err != nil {
		return "", fmt.Errorf("inspect new image config: %w", err)
	}
	spec := buildReplacementSpec(original, newImage, originalRef, plan, r.now())
	if err := r.engine.Stop(ctx, original.ID, "", defaultStopTimeout); err != nil {
		return "", fmt.Errorf("stop original container: %w", err)
	}
	renamed := original.Name + "-parachute-" + transactionSuffix(plan.ID)
	if err := r.engine.Rename(ctx, original.ID, renamed); err != nil {
		return "", fmt.Errorf("rename original container: %w", err)
	}
	replacementID, err := r.engine.Create(ctx, spec)
	if err != nil {
		return "", fmt.Errorf("create replacement: %w", err)
	}
	created, err := r.engine.Inspect(ctx, replacementID)
	if err != nil {
		return "", fmt.Errorf("inspect replacement: %w", err)
	}
	if created.ImageID != pulled.ID {
		return "", &ImageMismatchError{Approved: pulled.ID, Actual: created.ImageID}
	}
	if err := r.engine.Start(ctx, replacementID); err != nil {
		return "", fmt.Errorf("start replacement: %w", err)
	}
	return replacementID, nil
}

func (r *DockerRecreator) checkReproducible(ctx context.Context, original engine.Spec) error {
	if original.HostConfig != nil && original.HostConfig.AutoRemove {
		return &ConfigUnreproducibleError{Field: "AutoRemove"}
	}
	if original.Config != nil && original.Config.Labels[swarmServiceLabel] != "" {
		return &ConfigUnreproducibleError{Field: swarmServiceLabel}
	}
	if original.HostConfig != nil && original.HostConfig.NetworkMode.IsContainer() {
		if _, err := r.engine.Inspect(ctx, original.HostConfig.NetworkMode.ConnectedContainer()); err != nil && errdefs.IsNotFound(err) {
			return &ConfigUnreproducibleError{Field: "NetworkMode"}
		}
	}
	return nil
}

func findTarget(plan Plan, container string) (Target, bool) {
	for _, target := range plan.Targets {
		if target.Container == container {
			return target, true
		}
	}
	return Target{}, false
}

func transactionSuffix(id string) string {
	if len(id) < 6 {
		return id
	}
	return id[len(id)-6:]
}
