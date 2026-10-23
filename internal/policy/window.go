package policy

import (
	"fmt"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/schedule"
)

type WindowRule struct {
	cron     string
	duration time.Duration
	location *time.Location
}

func NewWindowRule(cron string, duration time.Duration, location *time.Location) *WindowRule {
	return &WindowRule{cron: cron, duration: duration, location: location}
}

func (r *WindowRule) Hold(s Subject) (domain.Hold, bool, error) {
	expression := s.Overrides.Window(r.cron)
	if expression == "" {
		return domain.Hold{}, false, nil
	}
	trigger, err := schedule.NewCronTrigger(expression, r.location)
	if err != nil {
		return domain.Hold{}, false, fmt.Errorf("maintenance window: %w", err)
	}
	duration := s.Overrides.WindowDuration(r.duration)
	if opened := trigger.First(s.Now.Add(-duration)); !opened.After(s.Now) && opened.Add(duration).After(s.Now) {
		return domain.Hold{}, false, nil
	}
	next := trigger.First(s.Now)
	return domain.Hold{Reason: "maintenance_window", Until: &next}, true, nil
}
