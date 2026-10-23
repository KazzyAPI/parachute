package app

import (
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/status"
)

func (c *Container) statusSource(hosts []pipeline.Host) status.Source {
	sources := make([]status.HostSource, 0, len(hosts))
	for _, host := range hosts {
		sources = append(sources, status.HostSource{Name: host.Name, Endpoint: host.Endpoint, Containers: host.Source})
	}
	return status.NewBuilder(sources, c.selector(), c.cfg.Armed)
}
