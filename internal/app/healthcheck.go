package app

import (
	"errors"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/health"
)

func (c *Container) Healthcheck() error {
	if health.NewMarker(c.cfg.RuntimeDir, c.deps.System, c.deps.System).Healthy() {
		return nil
	}
	return &ExitError{Code: domain.ExitFailures, Err: errors.New("no ready Parachute daemon is running")}
}
