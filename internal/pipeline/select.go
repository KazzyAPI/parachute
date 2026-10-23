package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
)

type Selector interface {
	Admit(containers []domain.Container) []domain.Container
	Select(containers []domain.Container) []domain.Container
}

type SelectStage struct {
	selector Selector
	logger   *slog.Logger
}

type imageLookup struct {
	labels map[string]string
	err    error
}

func NewSelectStage(selector Selector, logger *slog.Logger) *SelectStage {
	return &SelectStage{selector: selector, logger: logger}
}

func (s *SelectStage) Run(ctx context.Context, cycle *Cycle) error {
	for _, host := range cycle.ReachableHosts() {
		if err := s.selectHost(ctx, cycle, host); err != nil {
			return err
		}
	}
	return nil
}

func (s *SelectStage) selectHost(ctx context.Context, cycle *Cycle, host *HostRun) error {
	images := map[string]imageLookup{}
	var inspected []domain.Container
	var failed []*Candidate
	for _, container := range s.selector.Admit(host.Containers) {
		lookup, cached := images[container.ImageID]
		if !cached {
			lookup = s.inspect(ctx, host, container.ImageID)
			images[container.ImageID] = lookup
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var unreachable *engine.UnreachableError
		if errors.As(lookup.err, &unreachable) {
			host.Reachable = false
			s.logger.Error("host unreachable", "cycle", cycle.ID, "host", host.Host.Name, "endpoint", host.Host.Endpoint, "error", lookup.err.Error())
			return nil
		}
		if lookup.err != nil {
			result := domain.NewResult(host.Host.Name, container).Conclude(domain.StateFailed, reasonEngineError, lookup.err.Error())
			failed = append(failed, &Candidate{Container: container, Result: result})
			continue
		}
		container.ImageLabels = lookup.labels
		inspected = append(inspected, container)
	}
	host.Candidates = failed
	for _, container := range s.selector.Select(inspected) {
		host.Candidates = append(host.Candidates, &Candidate{Container: container, Result: domain.NewResult(host.Host.Name, container)})
	}
	return nil
}

func (s *SelectStage) inspect(ctx context.Context, host *HostRun, imageID string) imageLookup {
	image, found, err := host.Host.Source.Image(ctx, imageID)
	if err == nil && !found {
		err = fmt.Errorf("image %s of the container not found", imageID)
	}
	return imageLookup{labels: image.Labels, err: err}
}
