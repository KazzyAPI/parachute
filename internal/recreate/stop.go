package recreate

import (
	"strings"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/labels"
)

func stopSignal(container domain.Container, labels *labels.Reader, spec engine.Spec) string {
	if value, ok := labels.Value(container, "stop-signal"); ok {
		return value
	}
	if spec.Config != nil && spec.Config.StopSignal != "" {
		return spec.Config.StopSignal
	}
	return "SIGTERM"
}

func stopTimeout(spec engine.Spec, defaultTimeout time.Duration) time.Duration {
	if spec.Config != nil && spec.Config.StopTimeout != nil && *spec.Config.StopTimeout > 0 {
		return time.Duration(*spec.Config.StopTimeout) * time.Second
	}
	return defaultTimeout
}

func renamedName(name, txnID string) string {
	suffix := txnID
	if index := strings.LastIndex(txnID, "-"); index >= 0 {
		suffix = txnID[index+1:]
	}
	return name + "-parachute-" + suffix
}
