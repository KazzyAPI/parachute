package txn

import (
	"context"
	"time"
)

type Journal interface {
	Begin(ctx context.Context, record OpenRecord) error
	Advance(ctx context.Context, id string, state State) error
	Finish(ctx context.Context, id string, state State, result string, reason string) error
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
	Compare(ctx context.Context, original, replacement string) error
}

type Verdict struct {
	Passed   bool
	Reason   string
	Check    string
	Output   string
	Deadline time.Time
}

type Verifier interface {
	Verify(ctx context.Context, plan Plan, container string) (Verdict, error)
}

type Planner interface {
	Plan(ctx context.Context, graph DependencyGraph) ([]Plan, error)
}

type DependencyGraph struct {
	Host      string
	Stacks    []StackGraph
	SelfStack string
}

type StackGraph struct {
	Name       string
	Containers []string
	DependsOn  map[string][]string
}

type Plan struct {
	ID         string
	Host       string
	Stacks     []string
	Containers []string
	SelfUpdate bool
	Snapshot   string
}

type PlanIssue struct {
	Kind    string
	Stacks  []string
	Message string
}

type Hooks interface {
	PreUpdate(ctx context.Context, plan Plan) error
	PostUpdate(ctx context.Context, plan Plan) error
	PreCheck(ctx context.Context, plan Plan) error
	PostCheck(ctx context.Context, plan Plan) error
}

type NoOpHooks struct{}

func (NoOpHooks) PreUpdate(context.Context, Plan) error  { return nil }
func (NoOpHooks) PostUpdate(context.Context, Plan) error { return nil }
func (NoOpHooks) PreCheck(context.Context, Plan) error   { return nil }
func (NoOpHooks) PostCheck(context.Context, Plan) error  { return nil }
