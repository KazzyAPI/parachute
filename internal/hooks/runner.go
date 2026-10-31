package hooks

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/redact"
	"github.com/KazzyAPI/parachute/internal/txn"
)

const (
	defaultHookTimeout = 60 * time.Second
	maxHookOutput      = 4096
)

type runtimeReader interface {
	Runtime(ctx context.Context, id string) (engine.Runtime, error)
}

type execer interface {
	Exec(ctx context.Context, id string, cmd, env []string, limit time.Duration, maxOutput int64) (engine.ExecResult, error)
}

type specReader interface {
	Inspect(ctx context.Context, id string) (engine.Spec, error)
}

type Runner struct {
	labels   labelValuer
	runtime  runtimeReader
	exec     execer
	specs    specReader
	redactor *redact.Redactor
	logger   *slog.Logger
}

func New(labels labelValuer, runtime runtimeReader, exec execer, specs specReader, redactor *redact.Redactor, logger *slog.Logger) *Runner {
	return &Runner{labels: labels, runtime: runtime, exec: exec, specs: specs, redactor: redactor, logger: logger}
}

func (r *Runner) Run(ctx context.Context, name txn.HookName, container domain.Container) txn.HookOutcome {
	key := commandKey(name)
	if key == "" {
		return txn.HookOutcome{}
	}
	script, ok := r.labels.Value(container, key)
	if !ok || strings.TrimSpace(script) == "" {
		return txn.HookOutcome{}
	}
	state, err := r.runtime.Runtime(ctx, container.ID)
	if err != nil {
		r.logger.Debug("hook skipped", "hook", name, "container", container.Name, "error", err.Error())
		return txn.HookOutcome{}
	}
	if !state.Running {
		r.logger.Debug("hook skipped", "hook", name, "container", container.Name)
		return txn.HookOutcome{}
	}
	ceiling := txn.CeilingFrom(ctx)
	limit := boundTimeout(timeoutFor(name, r.labels, container), ceiling)
	if ceiling != nil && ceiling.Expired() {
		r.logger.Warn("transaction ceiling reached", "hook", name, "container", container.Name)
	}
	result, err := r.exec.Exec(ctx, container.ID, []string{"sh", "-c", script}, nil, limit, maxHookOutput)
	if output := result.Combined(); len(output) > 0 {
		r.logOutput(ctx, name, container, output)
	}
	timedOut := timedOut(err, ctx)
	if err != nil && !timedOut {
		r.logger.Warn("hook failed", "hook", name, "container", container.Name, "error", err.Error())
		return txn.HookOutcome{Ran: true, ExitCode: 1}
	}
	if timedOut || (ceiling != nil && ceiling.Expired() && err != nil) {
		r.logger.Warn("hook timed out", "hook", name, "container", container.Name)
		return txn.HookOutcome{Ran: true, TimedOut: true}
	}
	exit := result.ExitCode
	if name == txn.HookPreUpdate && exit == 75 {
		return txn.HookOutcome{Ran: true, ExitCode: exit, Deferred: true}
	}
	if name != txn.HookPreUpdate && exit == 75 {
		exit = 0
	}
	outcome := txn.HookOutcome{Ran: true, ExitCode: exit}
	if outcome.Failed() {
		r.logger.Warn("hook failed", "hook", name, "container", container.Name, "exit_code", result.ExitCode)
	}
	return outcome
}

func timedOut(err error, ctx context.Context) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return true
	}
	return err != nil && strings.Contains(err.Error(), "context deadline exceeded")
}

func (r *Runner) logOutput(ctx context.Context, name txn.HookName, container domain.Container, output []byte) {
	text := string(output)
	if spec, err := r.specs.Inspect(ctx, container.ID); err == nil {
		text = redactEnv(text, spec)
	}
	text = r.redactor.Redact(text)
	r.logger.Debug("hook output", "hook", name, "container", container.Name, "output", text)
}

func redactEnv(text string, spec engine.Spec) string {
	var values []string
	if spec.Config != nil {
		for _, pair := range spec.Config.Env {
			if _, value, ok := strings.Cut(pair, "="); ok && value != "" {
				values = append(values, value)
			}
		}
	}
	if len(values) == 0 {
		return text
	}
	return redact.New(redact.Static(values...)).Redact(text)
}
