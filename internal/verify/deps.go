package verify

import (
	"context"
	"time"

	"github.com/KazzyAPI/parachute/internal/engine"
)

type Clock interface {
	Now() time.Time
	Sleep(ctx context.Context, duration time.Duration) error
}

type Inspector interface {
	Inspect(ctx context.Context, id string) (engine.Spec, error)
	Runtime(ctx context.Context, id string) (engine.Runtime, error)
}

type Commander interface {
	Exec(ctx context.Context, id string, cmd, env []string, limit time.Duration, maxOutput int64) (engine.ExecResult, error)
}

type URLProber interface {
	Probe(ctx context.Context, transactionID, containerID, url string) (status int, body string, err error)
}

type Settings struct {
	Timeout time.Duration
	Stable  time.Duration
}

type LabelReader interface {
	Duration(containerID string, labels map[string]string, name string, fallback time.Duration) time.Duration
	String(containerID string, labels map[string]string, name string) (string, bool)
}
