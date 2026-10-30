package txn

import (
	"context"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type Outcome struct {
	State  State
	Result domain.State
	Reason Reason
}

type Journal interface {
	Begin(ctx context.Context, record OpenRecord) error
	Advance(ctx context.Context, id string, state State) error
	Finish(ctx context.Context, id string, outcome Outcome) error
	Open(ctx context.Context) ([]OpenRecord, error)
}

type Original struct {
	ID      string `json:"id"`
	Running bool   `json:"running"`
	Updated bool   `json:"updated,omitempty"`
}

type OpenRecord struct {
	ID         string
	Host       string
	Stacks     []string
	State      State
	Snapshot   string
	SelfUpdate bool
	Originals  []Original
}

type Target struct {
	Container string
	Image     string
	Digest    string
}

type Reason string

type Plan struct {
	ID            string
	Host          string
	Stacks        []string
	Targets       []Target
	SelfUpdate    bool
	Snapshot      string
	RemoveVolumes bool
}

type Snapshotter interface {
	Estimate(ctx context.Context, plan Plan) (int64, error)
	Take(ctx context.Context, plan Plan) (string, error)
	Validate(ctx context.Context, snapshot string) error
	Restore(ctx context.Context, snapshot string) error
	Discard(ctx context.Context, snapshot string) error
	Retain(ctx context.Context, host, stack string, keep int) error
	SkippedDestinations(ctx context.Context, snapshot string) (map[string][]string, error)
}

type Recreator interface {
	Reproducible(ctx context.Context, container string) error
	Replacement(ctx context.Context, plan Plan, id, name string) (string, error)
}

type Verdict struct {
	Passed bool
	Reason Reason
	Check  string
	Output string

	Deadline time.Duration
}

type Verifier interface {
	Verify(ctx context.Context, plan Plan, container string) (Verdict, error)
	VerifyBase(ctx context.Context, plan Plan, container string) (Verdict, error)
}
