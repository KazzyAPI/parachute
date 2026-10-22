package pipeline

import (
	"context"

	"github.com/KazzyAPI/parachute/internal/domain"
)

const reasonMonitorOnly = "monitor_only"

type ClassifyStage struct {
	labels      LabelSwitch
	monitorOnly SwitchSettings
}

func NewClassifyStage(labels LabelSwitch, monitorOnly SwitchSettings) *ClassifyStage {
	return &ClassifyStage{labels: labels, monitorOnly: monitorOnly}
}

func (s *ClassifyStage) Run(_ context.Context, cycle *Cycle) error {
	for _, host := range cycle.ReachableHosts() {
		for _, candidate := range host.Candidates {
			if candidate.Result.State == domain.StateWouldUpdate && s.labels.Switch(candidate.Container, "monitor-only", s.monitorOnly.Global, s.monitorOnly.LabelWins) {
				candidate.Result = candidate.Result.Conclude(domain.StateHeld, reasonMonitorOnly, "")
			}
		}
	}
	return nil
}
