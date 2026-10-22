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
			candidate.Result.State, candidate.Result.Reason = s.classify(candidate)
			candidate.Result.CandidateDigest = candidate.Finding.Candidate
		}
	}
	return nil
}

func (s *ClassifyStage) classify(candidate *Candidate) (domain.State, string) {
	finding := candidate.Finding
	if !finding.UpdateAvailable {
		return finding.State, finding.Reason
	}
	if s.labels.Switch(candidate.Container, "monitor-only", s.monitorOnly.Global, s.monitorOnly.LabelWins) {
		return domain.StateHeld, reasonMonitorOnly
	}
	return domain.StateWouldUpdate, ""
}
