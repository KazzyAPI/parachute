package recreate

import (
	"context"
	"fmt"

	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type Recreator struct {
	engine   Engine
	settings func() Settings
}

func NewRecreator(engine Engine, settings func() Settings) *Recreator {
	return &Recreator{engine: engine, settings: settings}
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

func (r *Recreator) Replacement(ctx context.Context, plan txn.Plan, container string) (string, error) {
	original, err := r.engine.Inspect(ctx, container)
	if err != nil {
		return "", err
	}
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
	return created, nil
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
