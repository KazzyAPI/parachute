package app

import (
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/selection"
)

func (c *Container) selector() *selection.Selector {
	return selection.NewSelector(c.cfg.Selection, c.labels, c.logger)
}

func (c *Container) selectStage() pipeline.Stage {
	return pipeline.NewSelectStage(c.selector())
}
