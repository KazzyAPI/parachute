package recreate

import (
	"context"
	"fmt"
	"sync"

	"github.com/moby/moby/api/types/container"

	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type Recreator struct {
	engine   Engine
	settings func() Settings
	lock     sync.Mutex
	replaced map[string]string
}

func NewRecreator(engine Engine, settings func() Settings) *Recreator {
	return &Recreator{engine: engine, settings: settings, replaced: map[string]string{}}
}

func (r *Recreator) Reproducible(ctx context.Context, container string) error {
	original, err := r.engine.Inspect(ctx, container)
	if err != nil {
		return err
	}
	if field, ok := checkReproducible(ctx, r.engine, original); !ok {
		return Failure{State: "failed", Reason: "config_unreproducible", Detail: field}
	}
	return nil
}

func (r *Recreator) Replacement(ctx context.Context, plan txn.Plan, id, name string) (string, error) {
	original, err := r.engine.Inspect(ctx, id)
	if err != nil {
		return "", err
	}
	original.Name = name
	return r.replaceSpec(ctx, plan, original, r.settings())
}

func (r *Recreator) replaceSpec(ctx context.Context, plan txn.Plan, original engine.Spec, settings Settings) (string, error) {
	if field, ok := checkReproducible(ctx, r.engine, original); !ok {
		return "", Failure{State: "failed", Reason: "config_unreproducible", Detail: field}
	}
	target := targetFor(plan, original.Name)
	if target.Image == "" {
		return "", fmt.Errorf("missing image for %s", original.Name)
	}
	oldImage, err := r.engine.ImageConfig(ctx, original.ImageID)
	if err != nil {
		return "", err
	}
	newImage, err := r.engine.ImageConfig(ctx, target.Image)
	if err != nil {
		return "", err
	}
	previousImage, err := previousImageRef(ctx, r.engine, original)
	if err != nil {
		return "", err
	}
	replacement := buildReplacement(original, oldImage, newImage, plan.ID, plan.Snapshot, previousImage, settings)
	replacement.Config.Image = target.Image
	r.followNetworkPeer(replacement.HostConfig)
	created, err := r.engine.Create(ctx, replacement)
	if err != nil {
		return "", err
	}
	createdSpec, err := r.engine.Inspect(ctx, created)
	if err != nil {
		_ = r.engine.Remove(ctx, created, false, true)
		return "", err
	}
	if field, ok := configsMatch(replacement, createdSpec); !ok {
		_ = r.engine.Remove(ctx, created, false, true)
		return "", Failure{State: "rolled_back", Reason: "config_unreproducible", Detail: field}
	}
	pulled, found, err := r.engine.Image(ctx, target.Image)
	if err != nil || !found || pulled.ID != createdSpec.ImageID {
		_ = r.engine.Remove(ctx, created, false, true)
		if err != nil {
			return "", err
		}
		return "", Failure{State: "rolled_back", Reason: "image_mismatch", Detail: fmt.Sprintf("%s %s", pulled.ID, createdSpec.ImageID)}
	}
	r.remember(original, created)
	return created, nil
}

func (r *Recreator) remember(original engine.Spec, created string) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.replaced[original.ID] = created
	r.replaced[original.Name] = created
}

func (r *Recreator) followNetworkPeer(host *container.HostConfig) {
	if host == nil || !host.NetworkMode.IsContainer() {
		return
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	if replacement, ok := r.replaced[host.NetworkMode.ConnectedContainer()]; ok {
		host.NetworkMode = container.NetworkMode("container:" + replacement)
	}
}

func targetFor(plan txn.Plan, container string) txn.Target {
	for _, target := range plan.Targets {
		if target.Container == container {
			return target
		}
	}
	return txn.Target{}
}

func previousImageRef(ctx context.Context, engine Engine, spec engine.Spec) (string, error) {
	image, found, err := engine.Image(ctx, spec.ImageID)
	if err != nil {
		return "", err
	}
	if !found {
		return spec.ImageID, nil
	}
	if spec.Config != nil && len(image.Digests) > 0 {
		return spec.Config.Image + "@" + image.Digests[0], nil
	}
	return spec.ImageID, nil
}
