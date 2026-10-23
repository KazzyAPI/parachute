package policy

import (
	"fmt"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/schedule"
)

type WindowRule struct {
	trigger  *schedule.CronTrigger
	duration time.Duration
}

func NewWindowRule(cron string, duration time.Duration, location *time.Location) (*WindowRule, error) {
	rule := &WindowRule{duration: duration}
	if cron == "" {
		return rule, nil
	}
	trigger, err := schedule.NewCronTrigger(cron, location)
	if err != nil {
		return nil, fmt.Errorf("--maintenance-window: %w", err)
	}
	rule.trigger = trigger
	return rule, nil
}

func (r *WindowRule) Hold(s Subject) (domain.Hold, bool, error) {
	trigger := s.Overrides.Window(r.trigger)
	if trigger == nil {
		return domain.Hold{}, false, nil
	}
	duration := s.Overrides.WindowDuration(r.duration)
	if opened := trigger.First(s.Now.Add(-duration)); !opened.After(s.Now) && opened.Add(duration).After(s.Now) {
		return domain.Hold{}, false, nil
	}
	next := trigger.First(s.Now)
	return domain.Hold{Reason: "maintenance_window", Until: &next}, true, nil
}
