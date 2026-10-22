package config

import (
	"fmt"
	"strings"
)

type EnvironmentReader struct {
	registry *Registry
	removed  *RemovedSettings
	suggest  *Suggester
}

func NewEnvironmentReader(registry *Registry, removed *RemovedSettings, suggest *Suggester) *EnvironmentReader {
	return &EnvironmentReader{registry: registry, removed: removed, suggest: suggest}
}

func (r *EnvironmentReader) Read(environ []string) (map[string]string, error) {
	values := map[string]string{}
	for _, entry := range environ {
		name, value, _ := strings.Cut(entry, "=")
		setting, known := r.registry.ByEnv(name)
		if known {
			values[setting.Label()] = value
			continue
		}
		if !strings.HasPrefix(name, "PARACHUTE_") {
			continue
		}
		if replacement, removed := r.removed.Env(name); removed {
			return nil, fmt.Errorf("%s was removed: %s", name, replacement)
		}
		return nil, r.suggest.Unknown(name, r.registry.EnvNames())
	}
	return values, nil
}
