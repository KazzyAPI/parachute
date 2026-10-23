package detect

import (
	"context"
	"errors"
	"slices"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/registry"
)

const (
	ReasonDigestPinned = "digest_pinned"
	ReasonLocalImage   = "local_image"
)

type ImageSource interface {
	Image(ctx context.Context, reference string) (engine.Image, bool, error)
}

type DigestSource interface {
	Digest(ctx context.Context, ref registry.Reference, credential *registry.Credential) (string, error)
}

type Detector struct {
	images   ImageSource
	registry DigestSource
}

func NewDetector(images ImageSource, digests DigestSource) *Detector {
	return &Detector{images: images, registry: digests}
}

func (d *Detector) Detect(ctx context.Context, result domain.Result, credentials registry.Credentials, noPull bool) (domain.Result, registry.Helper, error) {
	if registry.IsDigestPinned(result.Image) {
		return result.Conclude(domain.StateSkipped, ReasonDigestPinned, ""), registry.Helper{}, nil
	}
	ref, err := registry.ParseReference(result.Image)
	if err != nil {
		return result.Conclude(domain.StateSkipped, ReasonLocalImage, err.Error()), registry.Helper{}, nil
	}
	if noPull {
		result, err = d.detectLocally(ctx, result)
		return result, registry.Helper{}, err
	}
	image, found, err := d.images.Image(ctx, result.ImageID)
	if err != nil {
		return result, registry.Helper{}, err
	}
	if found && len(image.Digests) == 0 {
		return result.Conclude(domain.StateSkipped, ReasonLocalImage, ""), registry.Helper{}, nil
	}
	credential, helper := credentials.For(ref)
	digest, err := d.registry.Digest(ctx, ref, credential)
	if err != nil {
		return d.registryFailure(result, err), helper, nil
	}
	if slices.Contains(image.Digests, digest) || digest == result.ImageID {
		return result.Conclude(domain.StateFresh, "", ""), helper, nil
	}
	return result.Available(digest), helper, nil
}

func (d *Detector) detectLocally(ctx context.Context, result domain.Result) (domain.Result, error) {
	local, found, err := d.images.Image(ctx, result.Image)
	if err != nil {
		return result, err
	}
	if !found || local.ID == result.ImageID {
		return result.Conclude(domain.StateFresh, "", ""), nil
	}
	return result.Available(local.ID), nil
}

func (d *Detector) registryFailure(result domain.Result, err error) domain.Result {
	var registryErr *registry.Error
	if !errors.As(err, &registryErr) {
		return result.Conclude(domain.StateFailed, registry.ReasonUnreachable, err.Error())
	}
	if registryErr.Reason == registry.ReasonNotFound {
		return result.Conclude(domain.StateSkipped, ReasonLocalImage, err.Error())
	}
	return result.Conclude(domain.StateFailed, registryErr.Reason, err.Error())
}
