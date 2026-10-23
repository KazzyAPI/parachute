package pipeline

import (
	"context"
	"log/slog"

	"github.com/KazzyAPI/parachute/internal/engine"
)

type SelfMarker interface {
	Mark(hosts []engine.SelfHost) bool
	Contained() bool
}

type DiscoverStage struct {
	self      SelfMarker
	logger    *slog.Logger
	evaluated bool
}

func NewDiscoverStage(self SelfMarker, logger *slog.Logger) *DiscoverStage {
	return &DiscoverStage{self: self, logger: logger}
}

func (s *DiscoverStage) Run(ctx context.Context, cycle *Cycle) error {
	var listed []engine.SelfHost
	for _, host := range cycle.Hosts {
		containers, err := host.Host.Source.Containers(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			host.Unreachable(s.logger, cycle.ID, err)
			continue
		}
		host.Containers = containers
		listed = append(listed, engine.SelfHost{Local: engine.LocalEndpoint(host.Host.Endpoint), Containers: containers})
	}
	marked := s.self.Mark(listed)
	if len(listed) == 0 || s.evaluated {
		return nil
	}
	s.evaluated = true
	if !marked && s.self.Contained() {
		s.logger.Warn("running in a container whose ID cannot be established; running as not containerised", "cycle", cycle.ID)
	}
	return nil
}
