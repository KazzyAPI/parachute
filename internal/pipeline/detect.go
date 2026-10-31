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

type TargetResolver interface {
	Target(ctx context.Context, container domain.Container, credentials registry.Credentials) (string, error)
}

type SwitchSettings struct {
	Global    bool
	LabelWins bool
}

type DetectStage struct {
	credentials CredentialLoader
	labels      LabelSwitch
	noPull      SwitchSettings
	targets     TargetResolver
	logger      *slog.Logger
}

func NewDetectStage(credentials CredentialLoader, labels LabelSwitch, noPull SwitchSettings, targets TargetResolver, logger *slog.Logger) *DetectStage {
	return &DetectStage{credentials: credentials, labels: labels, noPull: noPull, targets: targets, logger: logger}
}

func (s *DetectStage) Run(ctx context.Context, cycle *Cycle) error {
	s.logger.Info("detect", "phase", "detect", "cycle", cycle.ID)
	credentials, err := s.credentials.Load()
	if err != nil {
		s.logger.Warn("registry credentials not loaded, using anonymous access", "cycle", cycle.ID, "error", err.Error())
	}
	warned := map[string]bool{}
	for _, host := range cycle.ReachableHosts() {
		for _, candidate := range host.Candidates {
			if candidate.Result.State != "" {
				continue
			}
			noPull := s.labels.Switch(candidate.Container, "no-pull", s.noPull.Global, s.noPull.LabelWins)
			target := s.target(ctx, cycle.ID, candidate.Container, credentials, noPull)
			detected := candidate.Result
			if target != "" {
				detected.Image = target
			}
			result, helper, err := host.Host.Detector.Detect(ctx, detected, credentials, noPull)
			result.Image = candidate.Result.Image
			if target != "" && result.State == domain.StateWouldUpdate {
				result.TargetImage = target
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if helper.Name != "" && !warned[helper.Registry] {
				warned[helper.Registry] = true
				s.logger.Warn("credential helpers are not supported, using anonymous access", "cycle", cycle.ID, "helper", helper.Name, "registry", helper.Registry)
			}
			var unreachable *engine.UnreachableError
			if errors.As(err, &unreachable) {
				host.Unreachable(s.logger, cycle.ID, err)
				host.Candidates = nil
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

func (s *DetectStage) target(ctx context.Context, cycleID string, container domain.Container, credentials registry.Credentials, noPull bool) string {
	if noPull {
		return ""
	}
	target, err := s.targets.Target(ctx, container, credentials)
	if err != nil {
		s.logger.Debug("cannot list tags to follow", "cycle", cycleID, "container", container.Name, "error", err.Error())
	}
	return target
}
