package graph

import (
	"context"

	"github.com/KazzyAPI/parachute/internal/pipeline"
)

type Stage struct {
	executor *Executor
	own      string
}

func NewStage(executor *Executor, own string) *Stage {
	return &Stage{executor: executor, own: own}
}

func (s *Stage) Run(ctx context.Context, cycle *pipeline.Cycle) error {
	if !cycle.Armed || s.executor == nil {
		return nil
	}
	for _, host := range cycle.ReachableHosts() {
		if err := s.executor.Run(ctx, host.Host.Name, host.Containers, host.Candidates, s.own); err != nil {
			return err
		}
	}
	return nil
}
