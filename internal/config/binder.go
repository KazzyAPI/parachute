package config

import (
	"errors"
	"fmt"
	"time"
)

type binder struct {
	registry *Registry
	syntax   *Syntax
	values   map[string]Value
	err      error
}

func (b *binder) raw(label string) string { return b.values[label].Raw }

func (b *binder) explicit(label string) bool { return b.values[label].Source >= SourceEnv }

func (b *binder) fail(label string, err error) {
	if err != nil && b.err == nil {
		b.err = fmt.Errorf("invalid value %q for %s: %w", b.raw(label), label, err)
	}
}

func (b *binder) boolean(label string) bool {
	value, err := b.syntax.Bool(b.raw(label))
	b.fail(label, err)
	return value
}

func (b *binder) integer(label string) int {
	value, err := b.syntax.Int(b.raw(label))
	b.fail(label, err)
	return value
}

func (b *binder) seconds(label string) time.Duration {
	return time.Duration(b.integer(label)) * time.Second
}

func (b *binder) list(label string) []string { return b.syntax.List(b.raw(label)) }

func (b *binder) relaxed() []string {
	var relaxed []string
	for _, setting := range b.registry.Settings() {
		if b.values[setting.Label()].Source == SourceProfile {
			relaxed = append(relaxed, setting.Label())
		}
	}
	return relaxed
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
