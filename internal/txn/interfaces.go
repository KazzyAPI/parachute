package txn

import (
	"context"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type Outcome struct {
	State  State
	Result domain.State
	Reason string
}

type Journal interface {
	Begin(ctx context.Context, record OpenRecord) error
	Advance(ctx context.Context, id string, state State) error
	Finish(ctx context.Context, id string, outcome Outcome) error
	Open(ctx context.Context) ([]OpenRecord, error)
}

type OpenRecord struct {
	ID         string
	Host       string
	Stacks     []string
	State      State
	Snapshot   string
	SelfUpdate bool
}

type Plan struct {
	ID         string
	Host       string
	Stacks     []string
	Containers []string
	SelfUpdate bool
	Snapshot   string
}

type Snapshotter interface {
	Estimate(ctx context.Context, plan Plan) (int64, error)
	Take(ctx context.Context, plan Plan) (string, error)
	Validate(ctx context.Context, snapshot string) error
	Restore(ctx context.Context, snapshot string) error
	Discard(ctx context.Context, snapshot string) error
	Retain(ctx context.Context, host, stack string, keep int) error
}

type Recreator interface {
	Replacement(ctx context.Context, plan Plan, container string) (string, error)
}

type Verdict struct {
	Passed bool
	Reason string
	Check  string
	Output string
}

type Verifier interface {
	Verify(ctx context.Context, plan Plan, container string) (Verdict, error)
}
