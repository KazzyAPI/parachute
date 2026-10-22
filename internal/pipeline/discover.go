package pipeline

import (
	"context"
	"log/slog"
)

type DiscoverStage struct {
	logger *slog.Logger
}

func NewDiscoverStage(logger *slog.Logger) *DiscoverStage {
	return &DiscoverStage{logger: logger}
}

func (s *DiscoverStage) Run(ctx context.Context, cycle *Cycle) error {
	for _, host := range cycle.Hosts {
		containers, err := host.Host.Source.Containers(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			host.Reachable = false
			s.logger.Error("host unreachable", "cycle", cycle.ID, "host", host.Host.Name, "endpoint", host.Host.Endpoint, "error", err.Error())
			continue
		}
		host.Containers = containers
	}
	return nil
}
