package app

import (
	"fmt"

	"github.com/KazzyAPI/parachute/internal/schedule"
)

func (c *Container) trigger() (schedule.Trigger, error) {
	if c.cfg.Schedule.Cron == "" {
		return schedule.NewIntervalTrigger(c.cfg.Schedule.Interval), nil
	}
	trigger, err := schedule.NewCronTrigger(c.cfg.Schedule.Cron, c.cfg.Schedule.Location)
	if err != nil {
		return nil, configurationError(fmt.Errorf("--schedule: %w", err))
	}
	return trigger, nil
}

func (c *Container) restoreTrigger() (schedule.Trigger, error) {
	trigger, err := schedule.NewCronTrigger(c.cfg.RestoreVerify.Schedule, c.cfg.Schedule.Location)
	if err != nil {
		return nil, configurationError(fmt.Errorf("--restore-verify-schedule: %w", err))
	}
	return trigger, nil
}
