package app

import "github.com/KazzyAPI/parachute/internal/schedule"

func (c *Container) trigger() (schedule.Trigger, error) {
	if c.cfg.Schedule.Cron == "" {
		return schedule.NewIntervalTrigger(c.cfg.Schedule.Interval), nil
	}
	trigger, err := schedule.NewCronTrigger(c.cfg.Schedule.Cron, c.cfg.Schedule.Location)
	if err != nil {
		return nil, configurationError(err)
	}
	return trigger, nil
}
