package verify

import (
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/labels"
)

type LabelAdapter struct {
	reader *labels.Reader
}

func NewLabelAdapter(reader *labels.Reader) *LabelAdapter {
	return &LabelAdapter{reader: reader}
}

func (a *LabelAdapter) Duration(_ string, labelMap map[string]string, name string, fallback time.Duration) time.Duration {
	container := domain.Container{Labels: labelMap}
	if value, ok := a.reader.Value(container, name); ok {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func (a *LabelAdapter) String(_ string, labelMap map[string]string, name string) (string, bool) {
	container := domain.Container{Labels: labelMap}
	return a.reader.Value(container, name)
}
