package app

import (
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/state"
)

func (c *Container) policyStage(_ *state.Store) pipeline.Stage {
	return pipeline.NewClassifyStage(c.labels,
		pipeline.SwitchSettings{Global: c.cfg.Selection.MonitorOnly, LabelWins: c.cfg.Selection.LabelTakePrecedence})
}
