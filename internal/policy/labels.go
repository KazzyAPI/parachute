package policy

import (
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/KazzyAPI/parachute/internal/config"
	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/schedule"
)

const labelPrefix = "parachute."

type LabelValuer interface {
	Value(c domain.Container, name string) (string, bool)
}

type Labels struct {
	labels   LabelValuer
	syntax   *config.Syntax
	location *time.Location
	settings map[string]config.Setting
}

func NewLabels(labels LabelValuer, syntax *config.Syntax, location *time.Location) *Labels {
	enum := func(choices ...string) config.Setting {
		return config.Setting{Kind: config.KindEnum, Default: choices[0], Choices: choices}
	}
	settings := map[string]config.Setting{
		"delay-days":                  {Kind: config.KindDecimal},
		"database":                    {Kind: config.KindBool},
		"approval":                    enum("auto", "required"),
		"tag-follow":                  enum("none", "patch", "minor"),
		"backup":                      enum("stop", "live", "none"),
		"maintenance-window":          {Kind: config.KindString},
		"maintenance-window-duration": {Kind: config.KindDuration},
	}
	for name, setting := range settings {
		setting.Name = labelPrefix + name
		settings[name] = setting
	}
	return &Labels{labels: labels, syntax: syntax, location: location, settings: settings}
}

func (l *Labels) Overrides(c domain.Container) (Overrides, error) {
	overrides := Overrides{values: map[string]config.Value{}}
	for _, name := range slices.Sorted(maps.Keys(l.settings)) {
		raw, ok := l.labels.Value(c, name)
		if !ok {
			continue
		}
		value, err := l.syntax.Parse(l.settings[name], raw)
		if err == nil && name == "maintenance-window" {
			if overrides.window, err = schedule.NewCronTrigger(raw, l.location); err != nil {
				err = fmt.Errorf("%s%s: %w", labelPrefix, name, err)
			}
		}
		if err != nil {
			return Overrides{}, &pipeline.InvalidLabelError{Label: labelPrefix + name, Err: err}
		}
		overrides.values[name] = value
	}
	return overrides, nil
}

type Overrides struct {
	values map[string]config.Value
	window *schedule.CronTrigger
}

func (o Overrides) raw(name string) string { return o.values[name].Raw }

func (o Overrides) has(name string) bool {
	_, ok := o.values[name]
	return ok
}

func (o Overrides) DelayDays(global float64) float64 {
	if o.has("delay-days") {
		return o.values["delay-days"].Decimal
	}
	return global
}

func (o Overrides) Database() (bool, bool) {
	return o.values["database"].Bool, o.has("database")
}

func (o Overrides) Approval() string { return o.raw("approval") }

func (o Overrides) Window(global *schedule.CronTrigger) *schedule.CronTrigger {
	if o.window != nil {
		return o.window
	}
	return global
}

func (o Overrides) WindowDuration(global time.Duration) time.Duration {
	if o.has("maintenance-window-duration") {
		return o.values["maintenance-window-duration"].Duration
	}
	return global
}
