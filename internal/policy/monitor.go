package policy

import "github.com/KazzyAPI/parachute/internal/domain"

type LabelSwitch interface {
	Switch(container domain.Container, name string, global, labelWins bool) bool
}

type MonitorRule struct {
	labels    LabelSwitch
	global    bool
	labelWins bool
}

func NewMonitorRule(labels LabelSwitch, global, labelWins bool) *MonitorRule {
	return &MonitorRule{labels: labels, global: global, labelWins: labelWins}
}

func (r *MonitorRule) Hold(s Subject) (domain.Hold, bool, error) {
	return domain.Hold{Reason: "monitor_only"}, r.labels.Switch(s.Container, "monitor-only", r.global, r.labelWins), nil
}
