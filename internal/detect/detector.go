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

func (d *Detector) Detect(ctx context.Context, result domain.Result, credentials registry.Credentials, noPull bool) (domain.Result, error) {
	if registry.IsDigestPinned(result.Image) {
		return result.Conclude(domain.StateSkipped, ReasonDigestPinned, ""), nil
	}
	ref, err := registry.ParseReference(result.Image)
	if err != nil {
		return result.Conclude(domain.StateSkipped, ReasonLocalImage, err.Error()), nil
	}
	if noPull {
		return d.detectLocally(ctx, result)
	}
	image, found, err := d.images.Image(ctx, result.ImageID)
	if err != nil {
		return result, err
	}
	if found && len(image.Digests) == 0 {
		return result.Conclude(domain.StateSkipped, ReasonLocalImage, ""), nil
	}
	credential, hasCredential := credentials.For(ref)
	var credentialPointer *registry.Credential
	if hasCredential {
		credentialPointer = &credential
	}
	digest, err := d.registry.Digest(ctx, ref, credentialPointer)
	if err != nil {
		return d.registryFailure(result, err), nil
	}
	if slices.Contains(image.Digests, digest) || digest == result.ImageID {
		return result.Conclude(domain.StateFresh, "", ""), nil
	}
	return result.Available(digest), nil
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
