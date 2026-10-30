package hooks

import (
	"strconv"
	"strings"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type labelValuer interface {
	Value(container domain.Container, name string) (string, bool)
}

func commandKey(name txn.HookName) string {
	switch name {
	case txn.HookPreCheck:
		return "lifecycle.pre-check"
	case txn.HookPreUpdate:
		return "lifecycle.pre-update"
	case txn.HookPostUpdate:
		return "lifecycle.post-update"
	case txn.HookPostCheck:
		return "lifecycle.post-check"
	default:
		return ""
	}
}

func timeoutFor(name txn.HookName, labels labelValuer, container domain.Container) time.Duration {
	switch name {
	case txn.HookPreUpdate:
		return minutesLabel(labels, container, "lifecycle.pre-update-timeout", defaultHookTimeout)
	case txn.HookPostUpdate:
		return minutesLabel(labels, container, "lifecycle.post-update-timeout", defaultHookTimeout)
	default:
		return defaultHookTimeout
	}
}

func minutesLabel(labels labelValuer, container domain.Container, name string, fallback time.Duration) time.Duration {
	raw, ok := labels.Value(container, name)
	if !ok {
		return fallback
	}
	raw = strings.TrimSpace(raw)
	if raw == "0" {
		return 0
	}
	minutes, err := strconv.Atoi(raw)
	if err != nil || minutes < 0 {
		return fallback
	}
	return time.Duration(minutes) * time.Minute
}

func boundTimeout(hook time.Duration, ceiling *txn.Ceiling) time.Duration {
	if ceiling == nil {
		if hook <= 0 {
			return defaultHookTimeout
		}
		return hook
	}
	remaining := ceiling.Remaining()
	if hook <= 0 {
		if remaining > 0 {
			return remaining
		}
		return time.Nanosecond
	}
	if remaining <= 0 {
		return time.Nanosecond
	}
	if remaining < hook {
		return remaining
	}
	return hook
}
