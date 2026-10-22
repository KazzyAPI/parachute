package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/pipeline"
)

type Clock interface {
	Now() time.Time
	Sleep(ctx context.Context, duration time.Duration) error
}

type Runner struct {
	pipeline *pipeline.Pipeline
	hosts    []pipeline.Host
	armed    bool
	clock    Clock
	logger   *slog.Logger
}

func NewRunner(cyclePipeline *pipeline.Pipeline, hosts []pipeline.Host, armed bool, clock Clock, logger *slog.Logger) *Runner {
	return &Runner{pipeline: cyclePipeline, hosts: hosts, armed: armed, clock: clock, logger: logger}
}

func (r *Runner) Run(ctx context.Context, id string) domain.Report {
	cycle := pipeline.NewCycle(id, r.clock.Now(), r.armed, r.hosts)
	r.logger.Info("cycle started", "cycle", id)
	if err := r.pipeline.Run(ctx, cycle); err != nil {
		r.logger.Error("cycle aborted", "cycle", id, "error", err.Error())
	}
	return cycle.Report
}
