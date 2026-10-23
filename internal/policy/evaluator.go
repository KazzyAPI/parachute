package policy

import (
	"context"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
)

const reasonApprovalRequired = "approval_required"

type Subject struct {
	Host      string
	Container domain.Container
	Result    domain.Result
	Now       time.Time
	Overrides Overrides
	Approved  bool
}

type Rule interface {
	Hold(s Subject) (domain.Hold, bool, error)
}

type Evaluator struct {
	labels      *Labels
	approvals   *Approvals
	relaxations *Relaxations
	rules       []Rule
}

func NewEvaluator(labels *Labels, approvals *Approvals, relaxations *Relaxations, rules ...Rule) *Evaluator {
	return &Evaluator{labels: labels, approvals: approvals, relaxations: relaxations, rules: rules}
}

func (e *Evaluator) Evaluate(_ context.Context, host string, c domain.Container, r domain.Result, now time.Time) (domain.Hold, bool, error) {
	overrides, err := e.labels.Overrides(c)
	if err != nil {
		return domain.Hold{}, false, err
	}
	e.relaxations.Log(c, overrides)
	if r.State != domain.StateWouldUpdate {
		return domain.Hold{}, false, nil
	}
	approved, err := e.approvals.Approved(host, c, r.CandidateDigest)
	if err != nil {
		return domain.Hold{}, false, err
	}
	subject := Subject{Host: host, Container: c, Result: r, Now: now, Overrides: overrides, Approved: approved}
	var first *domain.Hold
	for _, rule := range e.rules {
		hold, held, err := rule.Hold(subject)
		if err != nil {
			return domain.Hold{}, false, err
		}
		if held && first == nil {
			first = &hold
		}
	}
	if first == nil {
		return domain.Hold{}, false, nil
	}
	return *first, true, nil
}
