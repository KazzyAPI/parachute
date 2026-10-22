package app

import (
	"fmt"
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

func (s StartupSummary) Fields() []any {
	return []any{
		"version", s.Version, "hosts", s.hosts(), "schedule", s.Schedule, "scope", s.scope(),
		"armed", s.Settings.Armed, "profile", s.Settings.Profile, "relaxed", strings.Join(s.Settings.Relaxed, ","),
	}
}

func (s StartupSummary) Text() string {
	var text strings.Builder
	fmt.Fprintf(&text, "Parachute started\nversion: %s\nhosts: %s\nschedule: %s\nscope: %s\narmed: %t\nprofile: %s\n",
		s.Version, s.hosts(), s.Schedule, s.scope(), s.Settings.Armed, s.Settings.Profile)
	if len(s.Settings.Relaxed) > 0 {
		fmt.Fprintf(&text, "relaxed safety defaults: %s\n", strings.Join(s.Settings.Relaxed, ", "))
	}
	return text.String()
}
