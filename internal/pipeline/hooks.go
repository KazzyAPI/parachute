package pipeline

import (
	"context"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type PreCheckStage struct {
	hooks map[string]txn.Hooks
}

func NewPreCheckStage(hooks map[string]txn.Hooks) *PreCheckStage {
	return &PreCheckStage{hooks: hooks}
}

func (s *PreCheckStage) Run(ctx context.Context, cycle *Cycle) error {
	for _, host := range cycle.ReachableHosts() {
		runner := s.hooks[host.Host.Name]
		if runner == nil {
			runner = txn.NoHooks{}
		}
		for _, candidate := range host.Candidates {
			runner.Run(ctx, txn.HookPreCheck, candidate.Container)
		}
	}
	return nil
}

type PostCheckStage struct {
	hooks map[string]txn.Hooks
}

func NewPostCheckStage(hooks map[string]txn.Hooks) *PostCheckStage {
	return &PostCheckStage{hooks: hooks}
}

func (s *PostCheckStage) Run(ctx context.Context, cycle *Cycle) error {
	for _, host := range cycle.ReachableHosts() {
		runner := s.hooks[host.Host.Name]
		if runner == nil {
			runner = txn.NoHooks{}
		}
		for _, candidate := range host.Candidates {
			if postCheckInTransaction(candidate.Result) {
				continue
			}
			runner.Run(ctx, txn.HookPostCheck, candidate.Container)
		}
	}
	return nil
}

func postCheckInTransaction(result domain.Result) bool {
	if result.Transaction == "" {
		return false
	}
	return result.State == domain.StateUpdated || result.State == domain.StateRolledBack
}
