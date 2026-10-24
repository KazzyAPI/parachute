package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

type Runtime struct {
	Status       string
	Running      bool
	StartedAt    time.Time
	FinishedAt   time.Time
	RestartCount int
	Health       *Health
}

type Health struct {
	Status string
	Log    string
}

func (d *Docker) Runtime(ctx context.Context, id string) (Runtime, error) {
	inspected, err := d.client.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return Runtime{}, fmt.Errorf("inspect container %s: %w", id, err)
	}
	details := inspected.Container
	runtime := Runtime{
		Status:       string(details.State.Status),
		Running:      details.State.Running,
		RestartCount: details.RestartCount,
	}
	if details.State.StartedAt != "" {
		runtime.StartedAt, _ = time.Parse(time.RFC3339Nano, details.State.StartedAt)
	}
	if details.State.FinishedAt != "" {
		runtime.FinishedAt, _ = time.Parse(time.RFC3339Nano, details.State.FinishedAt)
	}
	if details.State.Health != nil {
		runtime.Health = &Health{Status: string(details.State.Health.Status), Log: lastHealthLog(details.State.Health.Log)}
	}
	return runtime, nil
}

func lastHealthLog(entries []*container.HealthcheckResult) string {
	if len(entries) == 0 {
		return ""
	}
	last := entries[len(entries)-1]
	if last == nil {
		return ""
	}
	return strings.TrimSpace(last.Output)
}
