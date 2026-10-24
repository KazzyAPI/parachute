package recreate

import (
	"context"
	"time"

	"github.com/KazzyAPI/parachute/internal/txn"
)

type Clock interface {
	Now() time.Time
	Sleep(ctx context.Context, duration time.Duration) error
}

type HealthVerifier struct {
	engine Engine
	clock  Clock
	wait   time.Duration
}

func NewHealthVerifier(engine Engine, clock Clock, wait time.Duration) *HealthVerifier {
	return &HealthVerifier{engine: engine, clock: clock, wait: wait}
}

func (v *HealthVerifier) Verify(ctx context.Context, plan txn.Plan, container string) (txn.Verdict, error) {
	deadline := v.clock.Now().Add(v.wait)
	for {
		spec, err := v.engine.Inspect(ctx, container)
		if err != nil {
			return txn.Verdict{}, err
		}
		if spec.Config == nil || spec.Config.Healthcheck == nil {
			return txn.Verdict{Passed: true}, nil
		}
		switch spec.HealthStatus {
		case "healthy":
			return txn.Verdict{Passed: true}, nil
		case "unhealthy":
			return txn.Verdict{Passed: false, Reason: txn.Reason("verify_failed"), Check: "healthcheck"}, nil
		}
		if v.clock.Now().After(deadline) {
			return txn.Verdict{Passed: false, Reason: txn.Reason("verify_timeout"), Check: spec.HealthStatus}, nil
		}
		if err := v.clock.Sleep(ctx, 500*time.Millisecond); err != nil {
			return txn.Verdict{}, err
		}
	}
}
