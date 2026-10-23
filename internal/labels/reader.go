package labels

import (
	"log/slog"
	"slices"
	"strings"

	"github.com/KazzyAPI/parachute/internal/domain"
)

const prefix = "parachute."

type Reader struct {
	known    map[string]bool
	cautious map[string]string
	logger   *slog.Logger
	warned   map[string]bool
}

func NewReader(logger *slog.Logger) *Reader {
	known := map[string]bool{}
	for _, name := range []string{
		"enable", "monitor-only", "no-pull", "scope", "self", "armed", "helper", "stop-signal", "depends-on",
		"lifecycle.pre-check", "lifecycle.pre-update", "lifecycle.post-update", "lifecycle.post-check",
		"lifecycle.pre-update-timeout", "lifecycle.post-update-timeout",
		"backup", "backup.exclude", "backup.dump", "verify.timeout", "verify.stable", "verify.cmd", "verify.url",
		"delay-days", "database", "approval", "tag-follow", "maintenance-window", "maintenance-window-duration",
		"transaction", "updated-at", "previous-image", "snapshot", "restore-verify",
	} {
		known[prefix+name] = true
	}
	cautious := map[string]string{prefix + "self": "true", prefix + "monitor-only": "true", prefix + "approval": "required"}
	return &Reader{known: known, cautious: cautious, logger: logger, warned: map[string]bool{}}
}

func (r *Reader) Value(container domain.Container, name string) (string, bool) {
	value, ok := container.Labels[prefix+name]
	if !ok || r.inherited(container, prefix+name) {
		return "", false
	}
	return value, true
}

func (r *Reader) inherited(container domain.Container, key string) bool {
	value := container.Labels[key]
	image, ok := container.ImageLabels[key]
	return ok && image == value && r.cautious[key] != normalised(value)
}

func normalised(value string) string {
	switch value = strings.ToLower(value); value {
	case "1":
		return "true"
	case "0":
		return "false"
	}
	return value
}

func (r *Reader) Is(container domain.Container, name string) bool {
	value, ok := r.Value(container, name)
	return ok && normalised(value) == "true"
}

func (r *Reader) WarnInherited(containers []domain.Container) {
	for _, container := range containers {
		var ignored []string
		for key := range container.Labels {
			if strings.HasPrefix(key, prefix) && r.inherited(container, key) {
				ignored = append(ignored, key)
			}
		}
		if len(ignored) > 0 {
			slices.Sort(ignored)
			r.logger.Warn("ignoring parachute labels inherited from the image", "container", container.Name, "labels", strings.Join(ignored, ","))
		}
	}
}

func (r *Reader) Bool(container domain.Container, name string) (bool, bool) {
	value, ok := r.Value(container, name)
	if !ok {
		return false, false
	}
	switch normalised(value) {
	case "true":
		return true, true
	case "false":
		return false, true
	}
	r.logger.Warn("ignoring label with an invalid boolean value", "container", container.Name, "label", prefix+name, "value", value)
	return false, false
}

func (r *Reader) Switch(container domain.Container, name string, global, labelWins bool) bool {
	label, set := r.Bool(container, name)
	if set && labelWins {
		return label
	}
	return global || label
}

func (r *Reader) WarnUnknown(containers []domain.Container) {
	warned := map[string]bool{}
	for _, container := range containers {
		for key := range container.Labels {
			if !strings.HasPrefix(key, prefix) || r.known[key] {
				continue
			}
			warning := container.ID + "\x00" + key
			if !r.warned[warning] {
				r.logger.Warn("unknown label ignored", "container", container.Name, "label", key)
			}
			warned[warning] = true
		}
	}
	r.warned = warned
}
