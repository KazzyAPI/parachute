package config

import (
	"errors"
	"fmt"
	"time"
)

type binder struct {
	registry *Registry
	values   map[string]Value
}

func (b *binder) raw(label string) string { return b.values[label].Raw }

func (b *binder) explicit(label string) bool { return b.values[label].Source >= SourceEnv }

func (b *binder) boolean(label string) bool { return b.values[label].Bool }

func (b *binder) seconds(label string) time.Duration {
	return time.Duration(b.values[label].Int) * time.Second
}

func (b *binder) integer(label string) int { return b.values[label].Int }

func (b *binder) decimal(label string) float64 { return b.values[label].Decimal }

func (b *binder) duration(label string) time.Duration { return b.values[label].Duration }

func (b *binder) list(label string) []string { return b.values[label].List }

func (b *binder) relaxations() []Relaxation {
	var relaxations []Relaxation
	for _, setting := range b.registry.Settings() {
		label := setting.Label()
		if b.values[label].Source == SourceProfile || b.explicit(label) && b.relaxes(label) {
			relaxations = append(relaxations, Relaxation{Setting: label, Value: b.raw(label)})
		}
	}
	return relaxations
}

func (b *binder) relaxes(label string) bool {
	switch label {
	case "backup":
		return b.raw(label) == "none"
	case "delay-days":
		return b.decimal(label) == 0
	case "protect-databases":
		return !b.boolean(label)
	case "tag-follow":
		return b.raw(label) != "none"
	case "allow-insecure-tcp":
		return b.boolean(label)
	}
	return false
}

func (b *binder) unimplemented() error {
	for _, setting := range b.registry.Settings() {
		if setting.Unimplemented == "" {
			continue
		}
		current := b.raw(setting.Label())
		if setting.Kind == KindBool {
			current = fmt.Sprint(b.boolean(setting.Label()))
		}
		if current != setting.Default {
			return errors.New(setting.Unimplemented)
		}
	}
	return nil
}
