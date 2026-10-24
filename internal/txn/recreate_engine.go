package txn

import (
	"context"
	"time"

	dockerspec "github.com/moby/docker-image-spec/specs-go/v1"

	"github.com/KazzyAPI/parachute/internal/engine"
)

type RecreateEngine interface {
	Inspect(ctx context.Context, id string) (engine.Spec, error)
	ImageConfig(ctx context.Context, reference string) (*dockerspec.DockerOCIImageConfig, error)
	Image(ctx context.Context, reference string) (engine.Image, bool, error)
	Pull(ctx context.Context, repository, digest string, auth engine.RegistryAuth) error
	Tag(ctx context.Context, source, target string) error
	Create(ctx context.Context, spec engine.Spec) (string, error)
	Rename(ctx context.Context, id, name string) error
	Stop(ctx context.Context, id, signal string, timeout time.Duration) error
	Start(ctx context.Context, id string) error
}
