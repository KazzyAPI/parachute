package status

import (
	"context"
	"time"

	"github.com/KazzyAPI/parachute/internal/state"
)

type Redactor interface {
	Redact(text string) string
}

type Source interface {
	Build(ctx context.Context, saved state.Document, generated time.Time) (Document, error)
}
