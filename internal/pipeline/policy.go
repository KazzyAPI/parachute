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
	Value string
	Err   error
}

func (e *InvalidLabelError) Error() string {
	return fmt.Sprintf("invalid value %q for label %s: %v", e.Value, e.Label, e.Err)
}

func (e *InvalidLabelError) Unwrap() error { return e.Err }

type PolicyStage struct {
	evaluator HoldEvaluator
	clock     Clock
}

func NewPolicyStage(evaluator HoldEvaluator, clock Clock) *PolicyStage {
	return &PolicyStage{evaluator: evaluator, clock: clock}
}

func (s *PolicyStage) Run(ctx context.Context, cycle *Cycle) error {
	now := s.clock.Now()
	for _, host := range cycle.ReachableHosts() {
		for _, candidate := range host.Candidates {
			candidate.Result.ContainerID = candidate.Container.ID
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
