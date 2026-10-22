package selection

import (
	"log/slog"
	"slices"
	"strings"

	"github.com/KazzyAPI/parachute/internal/config"
	"github.com/KazzyAPI/parachute/internal/domain"
)

type LabelReader interface {
	Value(container domain.Container, name string) (string, bool)
	Bool(container domain.Container, name string) (bool, bool)
	WarnUnknown(containers []domain.Container)
}

type Selector struct {
	settings config.Selection
	labels   LabelReader
	logger   *slog.Logger
	names    map[string]bool
}

func NewSelector(settings config.Selection, labels LabelReader, logger *slog.Logger) *Selector {
	names := map[string]bool{}
	for _, name := range settings.Names {
		names[strings.TrimPrefix(name, "/")] = true
	}
	return &Selector{settings: settings, labels: labels, logger: logger, names: names}
}

func (s *Selector) Select(containers []domain.Container) []domain.Container {
	s.warnUnmatchedNames(containers)
	var candidates []domain.Container
	for _, container := range containers {
		if s.isCandidate(container) {
			candidates = append(candidates, container)
		}
	}
	s.labels.WarnUnknown(candidates)
	return candidates
}

func (s *Selector) isCandidate(container domain.Container) bool {
	return !s.isOwn(container) &&
		s.stateAllowed(container.State) &&
		s.nameAllowed(container) &&
		s.enableAllowed(container) &&
		!slices.Contains(s.settings.Disabled, container.Name) &&
		s.scopeAllowed(container)
}

func (s *Selector) isOwn(container domain.Container) bool {
	helper, _ := s.labels.Bool(container, "helper")
	self, _ := s.labels.Bool(container, "self")
	return helper || self
}

func (s *Selector) stateAllowed(state string) bool {
	switch state {
	case "running":
		return true
	case "restarting":
		return s.settings.IncludeRestarting
	case "created", "exited":
		return s.settings.IncludeStopped
	}
	return false
}

func (s *Selector) nameAllowed(container domain.Container) bool {
	return len(s.names) == 0 || s.names[container.Name]
}

func (s *Selector) enableAllowed(container domain.Container) bool {
	enabled, set := s.labels.Bool(container, "enable")
	if s.settings.LabelEnable {
		return set && enabled
	}
	return !set || enabled
}

func (s *Selector) scopeAllowed(container domain.Container) bool {
	if !s.settings.ScopeSet {
		return true
	}
	scope, labelled := s.labels.Value(container, "scope")
	if s.settings.Scope == "none" {
		return !labelled
	}
	return labelled && scope == s.settings.Scope
}

func (s *Selector) warnUnmatchedNames(containers []domain.Container) {
	for name := range s.names {
		matched := slices.ContainsFunc(containers, func(container domain.Container) bool { return container.Name == name })
		if !matched {
			s.logger.Warn("no container matches the given name", "name", name)
		}
	}
}
