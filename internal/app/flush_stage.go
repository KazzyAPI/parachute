package app

import (
	"context"

	"github.com/KazzyAPI/parachute/internal/notify"
	"github.com/KazzyAPI/parachute/internal/pipeline"
)

type flushBeforeFinish struct {
	reporter *notify.Reporter
	finish   pipeline.Stage
}

func (s flushBeforeFinish) Run(ctx context.Context, cycle *pipeline.Cycle) error {
	if cycle.HandoverTicket != nil {
		s.reporter.Wait()
	}
	return s.finish.Run(ctx, cycle)
}
