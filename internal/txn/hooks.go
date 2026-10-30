package txn

import (
	"context"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type HookName string

const (
	HookPreCheck   HookName = "pre-check"
	HookPreUpdate  HookName = "pre-update"
	HookPostUpdate HookName = "post-update"
	HookPostCheck  HookName = "post-check"
)

type HookOutcome struct {
	Ran      bool
	TimedOut bool
	Deferred bool
	ExitCode int
}

func (o HookOutcome) Failed() bool {
	return o.TimedOut || (o.Ran && !o.Deferred && o.ExitCode != 0)
}

type Hooks interface {
	Run(ctx context.Context, name HookName, container domain.Container) HookOutcome
}

type NoHooks struct{}

func (NoHooks) Run(context.Context, HookName, domain.Container) HookOutcome {
	return HookOutcome{}
}
