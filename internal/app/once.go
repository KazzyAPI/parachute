package app

import (
	"context"
	"fmt"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type CycleRunner interface {
	Run(ctx context.Context, id string) domain.Report
}

type IDGenerator interface {
	New(started time.Time) (string, error)
}

type SingleCycle struct {
	ids    IDGenerator
	clock  Clock
	runner CycleRunner
}

func NewSingleCycle(ids IDGenerator, clock Clock, runner CycleRunner) *SingleCycle {
	return &SingleCycle{ids: ids, clock: clock, runner: runner}
}

func (s *SingleCycle) Run(ctx context.Context) error {
	id, err := s.ids.New(s.clock.Now())
	if err != nil {
		return err
	}
	if code := s.runner.Run(ctx, id).ExitCode(); code != domain.ExitSuccess {
		return &ExitError{Code: code, Err: fmt.Errorf("cycle %s finished with problems", id)}
	}
	return nil
}
