package policy

import (
	"log/slog"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type Relaxations struct {
	databases *Databases
	logger    *slog.Logger
	seen      map[string]bool
}

func NewRelaxations(databases *Databases, logger *slog.Logger) *Relaxations {
	return &Relaxations{databases: databases, logger: logger, seen: map[string]bool{}}
}

func (r *Relaxations) Log(c domain.Container, o Overrides) {
	for _, name := range r.relaxed(c, o) {
		key := c.ID + "\x00" + name
		if r.seen[key] {
			continue
		}
		r.seen[key] = true
		r.logger.Warn("safety default relaxed by label", "container", c.Name, "label", labelPrefix+name, "value", o.raw(name))
	}
}

func (r *Relaxations) relaxed(c domain.Container, o Overrides) []string {
	var names []string
	if o.raw("backup") == "none" {
		names = append(names, "backup")
	}
	if database, set := o.Database(); set && !database && r.databases.Recognised(c.ImageRef) {
		names = append(names, "database")
	}
	if o.Approval() == "auto" {
		names = append(names, "approval")
	}
	if o.has("delay-days") && o.DelayDays(1) == 0 {
		names = append(names, "delay-days")
	}
	if o.has("tag-follow") && o.raw("tag-follow") != "none" {
		names = append(names, "tag-follow")
	}
	return names
}
