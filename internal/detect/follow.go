package detect

import (
	"context"
	"fmt"
	"strings"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/registry"
)

type TagSource interface {
	Tags(ctx context.Context, ref registry.Reference, credential *registry.Credential) ([]string, error)
}

type LabelValuer interface {
	Value(container domain.Container, name string) (string, bool)
}

type TagFollower struct {
	tags   TagSource
	labels LabelValuer
	global string
}

func NewTagFollower(tags TagSource, labels LabelValuer, global string) *TagFollower {
	return &TagFollower{tags: tags, labels: labels, global: global}
}

func (f *TagFollower) Target(ctx context.Context, container domain.Container, credentials registry.Credentials) (string, error) {
	mode := f.global
	if label, set := f.labels.Value(container, "tag-follow"); set && (label == "none" || label == "patch" || label == "minor") {
		mode = label
	}
	if mode == "none" || registry.IsDigestPinned(container.ImageRef) {
		return "", nil
	}
	ref, err := registry.ParseReference(container.ImageRef)
	if err != nil {
		return "", fmt.Errorf("follow tags of %s: %w", container.Name, err)
	}
	if !registry.IsVersionTag(ref.Tag) || !strings.HasSuffix(container.ImageRef, ":"+ref.Tag) {
		return "", nil
	}
	credential, _ := credentials.For(ref)
	tags, err := f.tags.Tags(ctx, ref, credential)
	if err != nil {
		return "", err
	}
	tag, moved := registry.FollowTag(ref.Tag, mode, tags)
	if !moved {
		return "", nil
	}
	return strings.TrimSuffix(container.ImageRef, ref.Tag) + tag, nil
}
