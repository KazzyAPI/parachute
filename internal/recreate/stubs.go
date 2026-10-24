package recreate

import (
	"context"

	"github.com/KazzyAPI/parachute/internal/txn"
)

type NoopSnapshotter struct{}

func (NoopSnapshotter) Estimate(context.Context, txn.Plan) (int64, error) { return 0, nil }
func (NoopSnapshotter) Take(context.Context, txn.Plan) (string, error)    { return "none", nil }
func (NoopSnapshotter) Validate(context.Context, string) error            { return nil }
func (NoopSnapshotter) Restore(context.Context, string) error             { return nil }
func (NoopSnapshotter) Discard(context.Context, string) error             { return nil }
func (NoopSnapshotter) Retain(context.Context, string, string, int) error { return nil }
