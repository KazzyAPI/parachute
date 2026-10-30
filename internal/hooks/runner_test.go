package hooks

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/redact"
	"github.com/KazzyAPI/parachute/internal/txn"
	"github.com/moby/moby/api/types/container"
)

type fakeLabels map[string]string

func (f fakeLabels) Value(_ domain.Container, name string) (string, bool) {
	value, ok := f["parachute."+name]
	return value, ok
}

type fakeRuntime struct {
	running bool
	err     error
}

func (f fakeRuntime) Runtime(context.Context, string) (engine.Runtime, error) {
	if f.err != nil {
		return engine.Runtime{}, f.err
	}
	return engine.Runtime{Running: f.running, Status: "running"}, nil
}

type fakeExec struct {
	result engine.ExecResult
	err    error
	limit  time.Duration
}

func (f *fakeExec) Exec(_ context.Context, _ string, _ []string, _ []string, limit time.Duration, maxOutput int64) (engine.ExecResult, error) {
	f.limit = limit
	if maxOutput != maxHookOutput {
		return engine.ExecResult{}, context.Canceled
	}
	return f.result, f.err
}

type fakeSpecs struct {
	spec engine.Spec
}

func (f fakeSpecs) Inspect(context.Context, string) (engine.Spec, error) {
	return f.spec, nil
}

func testRunner(labels fakeLabels, runtime fakeRuntime, exec *fakeExec, specs fakeSpecs) (*Runner, *bytes.Buffer) {
	var logged bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return New(labels, runtime, exec, specs, redact.New(redact.Static("top-secret")), logger), &logged
}

func TestRunner_skips_when_not_running(t *testing.T) {
	runner, logged := testRunner(fakeLabels{"parachute.lifecycle.pre-check": "touch /x"}, fakeRuntime{running: false}, &fakeExec{}, fakeSpecs{})
	outcome := runner.Run(t.Context(), txn.HookPreCheck, domain.Container{Name: "a", ID: "1"})
	if outcome.Ran {
		t.Fatal("expected hook not to run")
	}
	if !strings.Contains(logged.String(), "hook skipped") {
		t.Fatalf("expected debug skip log, got %q", logged.String())
	}
}

func TestRunner_pre_update_75_defers(t *testing.T) {
	runner, logged := testRunner(
		fakeLabels{"parachute.lifecycle.pre-update": "exit 75"},
		fakeRuntime{running: true},
		&fakeExec{result: engine.ExecResult{ExitCode: 75}},
		fakeSpecs{},
	)
	outcome := runner.Run(t.Context(), txn.HookPreUpdate, domain.Container{Name: "a", ID: "1"})
	if !outcome.Deferred || outcome.ExitCode != 75 || outcome.Failed() {
		t.Fatalf("unexpected outcome %+v", outcome)
	}
	if strings.Contains(logged.String(), "hook failed") {
		t.Fatalf("defer must not warn: %q", logged.String())
	}
}

func TestRunner_post_check_75_succeeds(t *testing.T) {
	runner, logged := testRunner(
		fakeLabels{"parachute.lifecycle.post-check": "exit 75"},
		fakeRuntime{running: true},
		&fakeExec{result: engine.ExecResult{ExitCode: 75}},
		fakeSpecs{},
	)
	outcome := runner.Run(t.Context(), txn.HookPostCheck, domain.Container{Name: "a", ID: "1"})
	if outcome.Failed() || outcome.ExitCode != 0 {
		t.Fatalf("unexpected outcome %+v", outcome)
	}
	if strings.Contains(logged.String(), "hook failed") {
		t.Fatalf("post-check 75 must be silent: %q", logged.String())
	}
}

func TestRunner_warns_on_post_update_failure(t *testing.T) {
	runner, logged := testRunner(
		fakeLabels{"parachute.lifecycle.post-update": "exit 1"},
		fakeRuntime{running: true},
		&fakeExec{result: engine.ExecResult{ExitCode: 1}},
		fakeSpecs{},
	)
	outcome := runner.Run(t.Context(), txn.HookPostUpdate, domain.Container{Name: "a", ID: "1"})
	if !outcome.Failed() {
		t.Fatal("expected failure")
	}
	if !strings.Contains(logged.String(), "hook failed") || !strings.Contains(logged.String(), "exit_code=1") {
		t.Fatalf("expected warn with exit code, got %q", logged.String())
	}
}

func TestRunner_timeout_uses_label_minutes(t *testing.T) {
	exec := &fakeExec{result: engine.ExecResult{}}
	runner, _ := testRunner(
		fakeLabels{
			"parachute.lifecycle.pre-update":         "sleep 1",
			"parachute.lifecycle.pre-update-timeout": "2",
		},
		fakeRuntime{running: true},
		exec,
		fakeSpecs{},
	)
	runner.Run(t.Context(), txn.HookPreUpdate, domain.Container{Name: "a", ID: "1"})
	if exec.limit != 2*time.Minute {
		t.Fatalf("limit %s, want 2m", exec.limit)
	}
}

func TestRunner_zero_timeout_uses_ceiling(t *testing.T) {
	exec := &fakeExec{result: engine.ExecResult{}}
	runner, _ := testRunner(
		fakeLabels{
			"parachute.lifecycle.pre-update":         "sleep 1",
			"parachute.lifecycle.pre-update-timeout": "0",
		},
		fakeRuntime{running: true},
		exec,
		fakeSpecs{},
	)
	clock := func() time.Time { return time.Unix(0, 0) }
	ceiling := txn.NewCeiling(90*time.Second, clock)
	ceiling.Begin()
	ctx := txn.WithCeiling(t.Context(), ceiling)
	runner.Run(ctx, txn.HookPreUpdate, domain.Container{Name: "a", ID: "1"})
	if exec.limit != 90*time.Second {
		t.Fatalf("limit %s, want 90s", exec.limit)
	}
}

func TestRunner_redacts_env_in_debug_output(t *testing.T) {
	exec := &fakeExec{result: engine.ExecResult{Output: []byte("value hunter2-secret tail\n")}}
	spec := engine.Spec{Config: &container.Config{Env: []string{"API_KEY=hunter2-secret"}}}
	runner, logged := testRunner(
		fakeLabels{"parachute.lifecycle.post-update": `echo "$API_KEY"`},
		fakeRuntime{running: true},
		exec,
		fakeSpecs{spec: spec},
	)
	runner.Run(t.Context(), txn.HookPostUpdate, domain.Container{Name: "a", ID: "1"})
	if strings.Contains(logged.String(), "hunter2-secret") || !strings.Contains(logged.String(), "[REDACTED]") {
		t.Fatalf("expected redacted hook output log, got %q", logged.String())
	}
}

func TestBoundTimeout_defaults_without_ceiling(t *testing.T) {
	if got := boundTimeout(0, nil); got != defaultHookTimeout {
		t.Fatalf("got %s", got)
	}
}

func TestMinutesLabel_zero_means_ceiling_only(t *testing.T) {
	labels := fakeLabels{"parachute.lifecycle.pre-update-timeout": "0"}
	if got := minutesLabel(labels, domain.Container{}, "lifecycle.pre-update-timeout", defaultHookTimeout); got != 0 {
		t.Fatalf("got %s", got)
	}
}
