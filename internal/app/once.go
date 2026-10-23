package app

import (
	"context"
	"fmt"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/schedule"
)

type CycleRunner interface {
	Run(ctx context.Context, id string) domain.Report
}

type SingleCycle struct {
	gate   *schedule.Gate
	runner CycleRunner
}

func NewSingleCycle(gate *schedule.Gate, runner CycleRunner) *SingleCycle {
	return &SingleCycle{gate: gate, runner: runner}
}

func (s *SingleCycle) Run(ctx context.Context) error {
	report, err := s.gate.Run(ctx, s.runner.Run)
	if err != nil {
		return fmt.Errorf("start cycle: %w", err)
	}
	if code := report.ExitCode(); code != domain.ExitSuccess {
		return &ExitError{Code: code, Err: fmt.Errorf("cycle %s finished with problems", report.CycleID)}
	}
	return nil
}
