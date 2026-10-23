package status

import (
	"context"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/state"
)

type ContainerSource interface {
	Containers(ctx context.Context) ([]domain.Container, error)
}

type HostSource struct {
	Name       string
	Endpoint   string
	Containers ContainerSource
}

type Selector interface {
	Select(containers []domain.Container) []domain.Container
}

type Redactor interface {
	Redact(text string) string
}

type Source interface {
	Build(ctx context.Context, saved state.Document, generated time.Time) (Document, error)
}
