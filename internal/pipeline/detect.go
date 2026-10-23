package pipeline

import (
	"context"
	"errors"
	"log/slog"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/registry"
)

const reasonEngineError = "engine_error"

type CredentialLoader interface {
	Load() (registry.Credentials, error)
}

type LabelSwitch interface {
	Switch(container domain.Container, name string, global, labelWins bool) bool
}

type SwitchSettings struct {
	Global    bool
	LabelWins bool
}

type DetectStage struct {
	credentials CredentialLoader
	labels      LabelSwitch
	noPull      SwitchSettings
	logger      *slog.Logger
}

func NewDetectStage(credentials CredentialLoader, labels LabelSwitch, noPull SwitchSettings, logger *slog.Logger) *DetectStage {
	return &DetectStage{credentials: credentials, labels: labels, noPull: noPull, logger: logger}
}

func (s *DetectStage) Run(ctx context.Context, cycle *Cycle) error {
	credentials, err := s.credentials.Load()
	if err != nil {
		s.logger.Warn("registry credentials not loaded, using anonymous access", "cycle", cycle.ID, "error", err.Error())
	}
	for _, host := range cycle.ReachableHosts() {
		for _, candidate := range host.Candidates {
			if candidate.Result.State != "" {
				continue
			}
			noPull := s.labels.Switch(candidate.Container, "no-pull", s.noPull.Global, s.noPull.LabelWins)
			result, err := host.Host.Detector.Detect(ctx, candidate.Result, credentials, noPull)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			var unreachable *engine.UnreachableError
			if errors.As(err, &unreachable) {
				host.Reachable = false
				host.Candidates = nil
				s.logger.Error("host unreachable", "cycle", cycle.ID, "host", host.Host.Name, "endpoint", host.Host.Endpoint, "error", err.Error())
				break
			}
			if err != nil {
				result = result.Conclude(domain.StateFailed, reasonEngineError, err.Error())
			}
			candidate.Result = result
		}
	}
	return nil
}
