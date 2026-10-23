package schedule

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type CycleRunner interface {
	Run(ctx context.Context, id string) domain.Report
}

type Clock interface {
	Now() time.Time
	Sleep(ctx context.Context, duration time.Duration) error
}

type Scheduler struct {
	trigger  Trigger
	clock    Clock
	gate     *Gate
	runner   CycleRunner
	logger   *slog.Logger
	location *time.Location
}

func NewScheduler(trigger Trigger, clock Clock, gate *Gate, runner CycleRunner, logger *slog.Logger, location *time.Location) *Scheduler {
	return &Scheduler{trigger: trigger, clock: clock, gate: gate, runner: runner, logger: logger, location: location}
}

func (s *Scheduler) Run(ctx context.Context) error {
	var cycles sync.WaitGroup
	defer cycles.Wait()
	next := s.trigger.First(s.clock.Now())
	for {
		s.logger.Info("next cycle scheduled", "next", next.In(s.location).Format(time.RFC3339))
		if err := s.clock.Sleep(ctx, next.Sub(s.clock.Now())); err != nil {
			return err
		}
		cycles.Go(func() {
			if _, running, started := s.gate.Run(ctx, s.runner.Run); !started && running != "" {
				s.logger.Info("scheduled cycle skipped, previous cycle still running", "running_cycle", running)
			}
		})
		next = s.following(next)
	}
}

func (s *Scheduler) following(previous time.Time) time.Time {
	next := s.trigger.Next(previous)
	for now := s.clock.Now(); !next.After(now); {
		next = s.trigger.Next(next)
	}
	return next
}
