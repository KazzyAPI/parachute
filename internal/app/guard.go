package app

import (
	"context"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/pipeline"
)

type LabelReader interface {
	Is(container domain.Container, name string) bool
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
	return nil
}
