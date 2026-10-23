package app

import (
	"context"
	"errors"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/pipeline"
)

type LabelReader interface {
	Bool(container domain.Container, name string) (bool, bool)
}

type SelfMarker interface {
	Mark(hosts []engine.SelfHost) bool
}

type ArmedLabelGuard struct {
	host   pipeline.Host
	self   SelfMarker
	labels LabelReader
}

func NewArmedLabelGuard(host pipeline.Host, self SelfMarker, labels LabelReader) *ArmedLabelGuard {
	return &ArmedLabelGuard{host: host, self: self, labels: labels}
}

func (g *ArmedLabelGuard) Check(ctx context.Context) error {
	containers, err := g.host.Source.Containers(ctx)
	if err != nil {
		return unreachableError(err)
	}
	g.self.Mark([]engine.SelfHost{{Local: engine.LocalEndpoint(g.host.Endpoint), Containers: containers}})
	for _, container := range containers {
		if armed, _ := g.labels.Bool(container, "armed"); armed && container.Own {
			return configurationError(errors.New("updates are not implemented in this milestone (parachute.armed=true on own container)"))
		}
	}
	return nil
}
