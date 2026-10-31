package app

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/KazzyAPI/parachute/internal/config"
	"github.com/KazzyAPI/parachute/internal/pipeline"
)

type StartupSummary struct {
	Version  string
	Hosts    []pipeline.Host
	Schedule string
	Settings config.Config
}

func (s StartupSummary) scope() string {
	if !s.Settings.Selection.ScopeSet {
		return "all containers"
	}
	return s.Settings.Selection.Scope
}

func (s StartupSummary) hosts() string {
	entries := make([]string, 0, len(s.Hosts))
	for _, host := range s.Hosts {
		entries = append(entries, host.Name+"="+host.Endpoint)
	}
	return strings.Join(entries, ", ")
}

func (s StartupSummary) relaxed() []string {
	names := make([]string, 0, len(s.Settings.Relaxations))
	for _, relaxation := range s.Settings.Relaxations {
		names = append(names, relaxation.Setting)
	}
	return names
}

func (s StartupSummary) WarnRelaxations(logger *slog.Logger) {
	for _, relaxation := range s.Settings.Relaxations {
		logger.Warn("safety default relaxed", "setting", relaxation.Setting, "value", relaxation.Value)
	}
}

func (s StartupSummary) Fields() []any {
	return []any{
		"version", s.Version, "hosts", s.hosts(), "schedule", s.Schedule, "scope", s.scope(),
		"armed", s.Settings.Armed, "profile", s.Settings.Profile, "relaxed", strings.Join(s.relaxed(), ","),
	}
}

func (s StartupSummary) Text() string {
	var text strings.Builder
	fmt.Fprintf(&text, "Parachute started\nversion: %s\nhosts: %s\nschedule: %s\nscope: %s\narmed: %t\nprofile: %s\n",
		s.Version, s.hosts(), s.Schedule, s.scope(), s.Settings.Armed, s.Settings.Profile)
	if !s.Settings.Armed {
		text.WriteString("hint: reporting only, nothing is changed. Review the report, then restart with PARACHUTE_ARMED=true (or --armed) to apply updates\n")
	}
	if relaxed := s.relaxed(); len(relaxed) > 0 {
		fmt.Fprintf(&text, "relaxed safety defaults: %s\n", strings.Join(relaxed, ", "))
	}
	return text.String()
}
