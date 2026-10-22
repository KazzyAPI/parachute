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

type Finding struct {
	UpdateAvailable bool
	Candidate       string
	State           domain.State
	Reason          string
	Detail          string
}

type Detector struct {
	images   ImageSource
	registry DigestSource
}

func NewDetector(images ImageSource, digests DigestSource) *Detector {
	return &Detector{images: images, registry: digests}
}

func (d *Detector) Detect(ctx context.Context, container domain.Container, credentials registry.Credentials, noPull bool) (Finding, error) {
	if registry.IsDigestPinned(container.ImageRef) {
		return Finding{State: domain.StateSkipped, Reason: ReasonDigestPinned}, nil
	}
	ref, err := registry.ParseReference(container.ImageRef)
	if err != nil {
		return d.unresolvable(err), nil
	}
	if noPull {
		return d.detectLocally(ctx, container)
	}
	image, found, err := d.images.Image(ctx, container.ImageID)
	if err != nil {
		return Finding{}, err
	}
	if found && len(image.Digests) == 0 {
		return Finding{State: domain.StateSkipped, Reason: ReasonLocalImage}, nil
	}
	credential, hasCredential := credentials.For(ref)
	var credentialPointer *registry.Credential
	if hasCredential {
		credentialPointer = &credential
	}
	digest, err := d.registry.Digest(ctx, ref, credentialPointer)
	if err != nil {
		return d.registryFailure(err)
	}
	if slices.Contains(image.Digests, digest) || digest == container.ImageID {
		return Finding{State: domain.StateFresh}, nil
	}
	return Finding{UpdateAvailable: true, Candidate: digest}, nil
}

func (d *Detector) detectLocally(ctx context.Context, container domain.Container) (Finding, error) {
	local, found, err := d.images.Image(ctx, container.ImageRef)
	if err != nil {
		return Finding{}, err
	}
	if !found || local.ID == container.ImageID {
		return Finding{State: domain.StateFresh}, nil
	}
	return Finding{UpdateAvailable: true, Candidate: local.ID}, nil
}

func (d *Detector) unresolvable(err error) Finding {
	return Finding{State: domain.StateSkipped, Reason: ReasonLocalImage, Detail: err.Error()}
}

func (d *Detector) registryFailure(err error) (Finding, error) {
	var registryErr *registry.Error
	if !errors.As(err, &registryErr) {
		return Finding{State: domain.StateFailed, Reason: registry.ReasonUnreachable, Detail: err.Error()}, nil
	}
	if registryErr.Reason == registry.ReasonNotFound {
		return Finding{State: domain.StateSkipped, Reason: ReasonLocalImage, Detail: err.Error()}, nil
	}
	return Finding{State: domain.StateFailed, Reason: registryErr.Reason, Detail: err.Error()}, nil
}
