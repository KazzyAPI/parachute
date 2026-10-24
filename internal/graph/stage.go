package graph

import (
	"context"

	"github.com/KazzyAPI/parachute/internal/pipeline"
)

type Stage struct {
	executors map[string]*Executor
	own       string
}

func NewStage(executors map[string]*Executor, own string) *Stage {
	return &Stage{executors: executors, own: own}
}

func (s *Stage) Run(ctx context.Context, cycle *pipeline.Cycle) error {
	if !cycle.Armed {
		return nil
	}
	for _, host := range cycle.ReachableHosts() {
		executor := s.executors[host.Host.Name]
		if executor == nil {
			continue
		}
		if err := executor.Run(ctx, host.Host.Name, host.Containers, host.Candidates, s.own); err != nil {
			return err
		}
	}
	return nil
}
