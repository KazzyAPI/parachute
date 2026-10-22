package pipeline

import (
	"context"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type Selector interface {
	Select(containers []domain.Container) []domain.Container
}

type SelectStage struct {
	selector Selector
}

func NewSelectStage(selector Selector) *SelectStage {
	return &SelectStage{selector: selector}
}

func (s *SelectStage) Run(_ context.Context, cycle *Cycle) error {
	for _, host := range cycle.ReachableHosts() {
		for _, container := range s.selector.Select(host.Containers) {
			host.Candidates = append(host.Candidates, &Candidate{
				Container: container,
				Result:    domain.NewResult(host.Host.Name, container),
			})
		}
	}
	return nil
}
