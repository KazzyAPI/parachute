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

func ownContainerArms(self SelfMarker, labels LabelReader, hosts []engine.SelfHost) bool {
	if !self.Mark(hosts) {
		return false
	}
	for _, host := range hosts {
		for _, container := range host.Containers {
			if container.Own && labels.Is(container, "armed") {
				return true
			}
		}
	}
	return false
}

func (c *Container) armFromOwnLabel(ctx context.Context, hosts []connectedHost) {
	if c.cfg.Armed {
		return
	}
	var selfHosts []engine.SelfHost
	for _, host := range hosts {
		if host.docker == nil || host.err != nil {
			continue
		}
		containers, err := host.docker.Containers(ctx)
		if err != nil {
			continue
		}
		selfHosts = append(selfHosts, engine.SelfHost{Local: engine.LocalEndpoint(host.spec.Endpoint), Containers: containers})
	}
	c.cfg.Armed = ownContainerArms(c.self(), c.labels, selfHosts)
}
