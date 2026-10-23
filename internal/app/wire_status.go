package app

import (
	"github.com/KazzyAPI/parachute/internal/drift"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/status"
)

func (c *Container) statusSource(hosts []pipeline.Host) status.Source {
	return status.NewBuilder(hosts, pipeline.New(c.discoverStage(), c.selectStage()), c.redactor, c.cfg.Armed)
}

func (c *Container) driftStage() pipeline.Stage {
	return drift.NewStage(drift.NewDetector(c.deps.System))
}
