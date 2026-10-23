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
	Is(container domain.Container, name string) bool
	WarnUnknown(containers []domain.Container)
	WarnInherited(containers []domain.Container)
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

func (s *Selector) Admit(containers []domain.Container) []domain.Container {
	s.warnUnmatchedNames(containers)
	var admitted []domain.Container
	for _, container := range containers {
		if s.stateAllowed(container.State) && s.nameAllowed(container) &&
			!slices.Contains(s.settings.Disabled, container.Name) && s.ownLabelsAllow(container) {
			admitted = append(admitted, container)
		}
	}
	return admitted
}

func (s *Selector) ownLabelsAllow(container domain.Container) bool {
	if s.settings.LabelEnable && !s.labels.Is(container, "enable") {
		return false
	}
	if s.settings.ScopeSet && s.settings.Scope != "none" && !s.scopeAllowed(container) {
		return false
	}
	return container.Own || !s.labels.Is(container, "self")
}

func (s *Selector) Select(containers []domain.Container) []domain.Container {
	var candidates []domain.Container
	for _, container := range containers {
		if s.isCandidate(container) {
			candidates = append(candidates, container)
		}
	}
	s.labels.WarnUnknown(candidates)
	s.labels.WarnInherited(candidates)
	return candidates
}

func (s *Selector) isCandidate(container domain.Container) bool {
	helper, _ := s.labels.Bool(container, "helper")
	return !helper && !s.foreignSelf(container) && s.enableAllowed(container) && s.scopeAllowed(container)
}

func (s *Selector) foreignSelf(container domain.Container) bool {
	self, _ := s.labels.Bool(container, "self")
	return self && !container.Own
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
