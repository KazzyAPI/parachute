package recreate

import (
	"context"
	"time"

	dockerspec "github.com/moby/docker-image-spec/specs-go/v1"

	"github.com/KazzyAPI/parachute/internal/engine"
)

type Engine interface {
	Inspect(ctx context.Context, id string) (engine.Spec, error)
	Image(ctx context.Context, reference string) (engine.Image, bool, error)
	ImageConfig(ctx context.Context, reference string) (*dockerspec.DockerOCIImageConfig, error)
	Pull(ctx context.Context, repository, digest string, auth engine.RegistryAuth) error
	Tag(ctx context.Context, source, target string) error
	Create(ctx context.Context, spec engine.Spec) (string, error)
	Start(ctx context.Context, id string) error
	Stop(ctx context.Context, id, signal string, timeout time.Duration) error
	Rename(ctx context.Context, id, name string) error
	Remove(ctx context.Context, id string, anonymousVolumes, force bool) error
	RemoveImage(ctx context.Context, reference string) error
}
