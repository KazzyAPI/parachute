package app

import (
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/selection"
)

func (c *Container) selector() *selection.Selector {
	return selection.NewSelector(c.cfg.Selection, c.labels, c.logger)
}

func (c *Container) self() engine.Self {
	return engine.ProbeSelf(c.deps.System)
}

func (c *Container) discoverStage() pipeline.Stage {
	return pipeline.NewDiscoverStage(c.self(), c.logger)
}

func (c *Container) selectStage() pipeline.Stage {
	return pipeline.NewSelectStage(c.selector(), c.logger)
}
