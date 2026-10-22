package schedule

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type CycleRunner interface {
	RunCycle(ctx context.Context, id string)
}

type Clock interface {
	Now() time.Time
	Sleep(ctx context.Context, duration time.Duration) error
}

type IDGenerator interface {
	New(started time.Time) (string, error)
}

type Scheduler struct {
	trigger  Trigger
	clock    Clock
	ids      IDGenerator
	runner   CycleRunner
	logger   *slog.Logger
	location *time.Location
	lock     sync.Mutex
	running  string
}

func NewScheduler(trigger Trigger, clock Clock, ids IDGenerator, runner CycleRunner, logger *slog.Logger, location *time.Location) *Scheduler {
	return &Scheduler{trigger: trigger, clock: clock, ids: ids, runner: runner, logger: logger, location: location}
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
		if running := s.current(); running != "" {
			s.logger.Info("scheduled cycle skipped, previous cycle still running", "running_cycle", running)
		} else if err := s.start(ctx, &cycles); err != nil {
			return err
		}
		next = s.trigger.Next(next)
	}
}

func (s *Scheduler) start(ctx context.Context, cycles *sync.WaitGroup) error {
	id, err := s.ids.New(s.clock.Now())
	if err != nil {
		return err
	}
	s.setRunning(id)
	cycles.Go(func() {
		defer s.setRunning("")
		s.runner.RunCycle(ctx, id)
	})
	return nil
}

func (s *Scheduler) current() string {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.running
}

func (s *Scheduler) setRunning(id string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.running = id
}
