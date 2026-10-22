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

func (b *binder) list(label string) []string { return b.values[label].List }

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
