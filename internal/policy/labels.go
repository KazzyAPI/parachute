package policy

import (
	"errors"
	"maps"
	"slices"
	"time"

	"github.com/KazzyAPI/parachute/internal/config"
	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/schedule"
)

const labelPrefix = "parachute."

type Labels struct {
	syntax   *config.Syntax
	location *time.Location
	settings map[string]config.Setting
	cautious map[string]string
}

func NewLabels(syntax *config.Syntax, location *time.Location) *Labels {
	enum := func(choices ...string) config.Setting {
		return config.Setting{Kind: config.KindEnum, Default: choices[0], Choices: choices}
	}
	return &Labels{syntax: syntax, location: location, settings: map[string]config.Setting{
		"delay-days":                  {Kind: config.KindDecimal},
		"database":                    {Kind: config.KindBool},
		"approval":                    enum("auto", "required"),
		"tag-follow":                  enum("none", "patch", "minor"),
		"backup":                      enum("stop", "live", "none"),
		"maintenance-window":          {Kind: config.KindString},
		"maintenance-window-duration": {Kind: config.KindDuration},
	}, cautious: map[string]string{"approval": "required"}}
}

func (l *Labels) Overrides(c domain.Container) (Overrides, error) {
	values := map[string]config.Value{}
	for _, name := range slices.Sorted(maps.Keys(l.settings)) {
		raw, ok := l.value(c, name)
		if !ok {
			continue
		}
		value, err := l.syntax.Parse(l.settings[name], raw)
		if err == nil && name == "maintenance-window" {
			_, err = schedule.NewCronTrigger(raw, l.location)
		}
		if err != nil {
			return Overrides{}, &pipeline.InvalidLabelError{Label: labelPrefix + name, Value: raw, Err: errors.Unwrap(err)}
		}
		values[name] = value
	}
	return Overrides{values: values}, nil
}

func (l *Labels) value(c domain.Container, name string) (string, bool) {
	key := labelPrefix + name
	value, ok := c.Labels[key]
	if image, baked := c.ImageLabels[key]; ok && baked && image == value && l.cautious[name] != value {
		return "", false
	}
	return value, ok
}

type Overrides struct {
	values map[string]config.Value
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

func (o Overrides) Window(global string) string {
	if o.has("maintenance-window") {
		return o.raw("maintenance-window")
	}
	return global
}

func (o Overrides) WindowDuration(global time.Duration) time.Duration {
	if o.has("maintenance-window-duration") {
		return o.values["maintenance-window-duration"].Duration
	}
	return global
}
