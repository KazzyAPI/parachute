package pipeline

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
)

const reasonInvalidLabel = "invalid_label"

type HoldEvaluator interface {
	Evaluate(ctx context.Context, host string, c domain.Container, r domain.Result, now time.Time) (domain.Hold, bool, error)
}

type InvalidLabelError struct {
	Label string
	Err   error
}

func (e *InvalidLabelError) Error() string { return e.Err.Error() }

func (e *InvalidLabelError) Unwrap() error { return e.Err }

type RecordPruner interface {
	Prune(host string, containers []domain.Container) error
}

type PolicyStage struct {
	evaluator HoldEvaluator
	pruner    RecordPruner
	clock     Clock
}

func NewPolicyStage(evaluator HoldEvaluator, pruner RecordPruner, clock Clock) *PolicyStage {
	return &PolicyStage{evaluator: evaluator, pruner: pruner, clock: clock}
}

func (s *PolicyStage) Run(ctx context.Context, cycle *Cycle) error {
	now := s.clock.Now()
	for _, host := range cycle.ReachableHosts() {
		if err := s.pruner.Prune(host.Host.Name, host.Containers); err != nil {
			return fmt.Errorf("prune policy records of host %s: %w", host.Host.Name, err)
		}
		for _, candidate := range host.Candidates {
			hold, held, err := s.evaluator.Evaluate(ctx, host.Host.Name, candidate.Container, candidate.Result, now)
			var invalid *InvalidLabelError
			switch {
			case errors.As(err, &invalid):
				if state := candidate.Result.State; state == domain.StateFresh || state == domain.StateWouldUpdate {
					candidate.Result = candidate.Result.Conclude(domain.StateFailed, reasonInvalidLabel, err.Error())
				}
			case err != nil:
				return fmt.Errorf("evaluate policies for %s: %w", candidate.Container.Name, err)
			case held && candidate.Result.State == domain.StateWouldUpdate:
				candidate.Result = candidate.Result.Conclude(domain.StateHeld, hold.Reason, hold.Detail)
				candidate.Result.HoldUntil = hold.Until
			}
		}
	}
	return nil
}
