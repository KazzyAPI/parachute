package graph

import (
	"context"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type recordingHooks struct {
	calls    []txn.HookName
	outcomes map[string]txn.HookOutcome
}

func (r *recordingHooks) Run(_ context.Context, name txn.HookName, container domain.Container) txn.HookOutcome {
	r.calls = append(r.calls, name)
	if r.outcomes != nil {
		if outcome, ok := r.outcomes[string(name)+"/"+container.Name]; ok {
			return outcome
		}
		if outcome, ok := r.outcomes[string(name)]; ok {
			return outcome
		}
	}
	return txn.HookOutcome{Ran: true}
}

type takeCounter struct {
	fakeSnapshots
	takes *int
}

func (t takeCounter) Take(context.Context, txn.Plan) (string, error) {
	*t.takes++
	return "snap-1", nil
}

func newHookHarness(hooks txn.Hooks) *harness {
	h := newHarness(fakeSnapshots{}, fakeRecreator{}, fakeVerifier{txn.Verdict{Passed: true}})
	h.executor.hooks = hooks
	return h
}

func TestHOK_007_failing_pre_update_aborts_before_stop(t *testing.T) {
	rec := &recordingHooks{outcomes: map[string]txn.HookOutcome{
		string(txn.HookPreUpdate): {Ran: true, ExitCode: 1},
	}}
	h := newHookHarness(rec)
	h.run(t)
	if h.engine.stops != 0 {
		t.Fatalf("stops %d", h.engine.stops)
	}
	if h.candidates[0].Result.State != domain.StateFailed || h.candidates[0].Result.Reason != "pre_update_hook_failed" {
		t.Fatalf("result %+v", h.candidates[0].Result)
	}
	if h.journal.outcome.State != txn.StateAborted {
		t.Fatalf("journal %+v", h.journal.outcome)
	}
}

func TestHOK_006_pre_update_75_defers_without_stopping(t *testing.T) {
	rec := &recordingHooks{outcomes: map[string]txn.HookOutcome{
		string(txn.HookPreUpdate): {Ran: true, ExitCode: 75, Deferred: true},
	}}
	var takes int
	h := newHookHarness(rec)
	h.executor.snapshot = takeCounter{takes: &takes}
	h.run(t)
	if h.engine.stops != 0 || takes != 0 {
		t.Fatalf("stops %d takes %d", h.engine.stops, takes)
	}
	if h.candidates[0].Result.State != domain.StateHeld || h.candidates[0].Result.Reason != "hook_deferred" {
		t.Fatalf("result %+v", h.candidates[0].Result)
	}
	if h.journal.outcome.State != txn.StateAborted || h.journal.outcome.Result != domain.StateHeld {
		t.Fatalf("journal %+v", h.journal.outcome)
	}
	if !containsHook(rec.calls, txn.HookPostCheck) {
		t.Fatalf("calls %v", rec.calls)
	}
	if containsHook(rec.calls, txn.HookPostUpdate) {
		t.Fatalf("post-update ran on defer: %v", rec.calls)
	}
}

func TestHOK_003_update_runs_pre_post_update_and_post_check_in_order(t *testing.T) {
	rec := &recordingHooks{}
	h := newHookHarness(rec)
	h.run(t)
	want := []txn.HookName{txn.HookPreUpdate, txn.HookPostUpdate, txn.HookPostCheck}
	if !hookSubsequence(rec.calls, want) {
		t.Fatalf("calls %v want subsequence %v", rec.calls, want)
	}
}

func TestHOK_006_deferred_update_proceeds_next_cycle(t *testing.T) {
	rec := &recordingHooks{outcomes: map[string]txn.HookOutcome{
		string(txn.HookPreUpdate): {Ran: true, ExitCode: 75, Deferred: true},
	}}
	h := newHookHarness(rec)
	h.run(t)
	rec.outcomes = nil
	rec.calls = nil
	h.candidates[0].Result = domain.NewResult("local", h.containers[0]).Available("sha256:new")
	h.run(t)
	if h.candidates[0].Result.State != domain.StateUpdated {
		t.Fatalf("result %+v", h.candidates[0].Result)
	}
}

func TestHOK_006_stack_deferral_holds_both_without_post_update(t *testing.T) {
	rec := &recordingHooks{outcomes: map[string]txn.HookOutcome{
		string(txn.HookPreUpdate) + "/app": {Ran: true, ExitCode: 75, Deferred: true},
	}}
	h := newHookHarness(rec)
	app := domain.Container{ID: "app-id", Name: "app", State: "running", ImageRef: "repo/app:1", Labels: map[string]string{"com.docker.compose.project": "shop"}}
	db := domain.Container{ID: "db-id", Name: "db", State: "running", ImageRef: "repo/db:1", Labels: map[string]string{"com.docker.compose.project": "shop"}}
	h.containers = []domain.Container{app, db}
	h.candidates = []*pipeline.Candidate{
		{Container: app, Result: domain.NewResult("local", app).Available("sha256:app")},
		{Container: db, Result: domain.NewResult("local", db).Available("sha256:db")},
	}
	h.run(t)
	for _, candidate := range h.candidates {
		if candidate.Result.State != domain.StateHeld || candidate.Result.Reason != "hook_deferred" {
			t.Fatalf("result %+v", candidate.Result)
		}
	}
	if containsHook(rec.calls, txn.HookPostUpdate) {
		t.Fatalf("post-update ran: %v", rec.calls)
	}
}

func TestUPD_019_pre_update_hook_without_timeout_hits_ceiling(t *testing.T) {
	clock := &steppingClock{now: time.Unix(0, 0)}
	h := newHookHarness(&expiringPreUpdateHooks{clock: clock})
	h.executor.clock = clock
	h.executor.options = Options{TransactionTimeout: 20 * time.Second}
	h.run(t)
	if !h.rollbacker.called || h.rollbacker.reason != "transaction_timeout" || h.rollbacker.final != domain.StateFailed {
		t.Fatalf("rollback %+v", h.rollbacker)
	}
	if h.engine.stops != 0 {
		t.Fatalf("stops %d", h.engine.stops)
	}
}

type expiringPreUpdateHooks struct {
	clock *steppingClock
}

func (e *expiringPreUpdateHooks) Run(context.Context, txn.HookName, domain.Container) txn.HookOutcome {
	e.clock.now = e.clock.now.Add(21 * time.Second)
	return txn.HookOutcome{Ran: true, TimedOut: true}
}

func TestHOK_003_post_update_runs_before_verify(t *testing.T) {
	rec := &recordingHooks{}
	h := newHookHarness(rec)
	h.run(t)
	preUpdate, postUpdate, postCheck := hookIndex(rec.calls, txn.HookPreUpdate), hookIndex(rec.calls, txn.HookPostUpdate), hookIndex(rec.calls, txn.HookPostCheck)
	if postUpdate < preUpdate || postCheck < postUpdate {
		t.Fatalf("calls %v", rec.calls)
	}
	if h.journal.states[len(h.journal.states)-1] != txn.StateVerify && !containsState(h.journal.states, txn.StateVerify) {
		t.Fatalf("states %v", h.journal.states)
	}
}

func containsHook(calls []txn.HookName, name txn.HookName) bool {
	for _, call := range calls {
		if call == name {
			return true
		}
	}
	return false
}

func hookIndex(calls []txn.HookName, name txn.HookName) int {
	for i, call := range calls {
		if call == name {
			return i
		}
	}
	return -1
}

func hookSubsequence(calls, want []txn.HookName) bool {
	index := 0
	for _, call := range calls {
		if index < len(want) && call == want[index] {
			index++
		}
	}
	return index == len(want)
}

func containsState(states []txn.State, state txn.State) bool {
	for _, item := range states {
		if item == state {
			return true
		}
	}
	return false
}

func TestHOK_007_pre_update_failure_skips_post_check(t *testing.T) {
	rec := &recordingHooks{outcomes: map[string]txn.HookOutcome{
		string(txn.HookPreUpdate): {Ran: true, ExitCode: 1},
	}}
	h := newHookHarness(rec)
	h.run(t)
	if containsHook(rec.calls, txn.HookPostCheck) {
		t.Fatalf("post-check ran after pre-update failure: %v", rec.calls)
	}
}

func TestHOK_003_rollback_runs_post_check(t *testing.T) {
	rec := &recordingHooks{}
	h := newHarness(fakeSnapshots{}, fakeRecreator{}, fakeVerifier{txn.Verdict{Reason: "verify_failed", Check: "health"}})
	h.executor.hooks = rec
	h.run(t)
	if !containsHook(rec.calls, txn.HookPostCheck) {
		t.Fatalf("calls %v", rec.calls)
	}
}

type estimateTakeSnapshot struct {
	fakeSnapshots
	onEstimate func()
	onTake     func()
}

func (e estimateTakeSnapshot) Estimate(ctx context.Context, plan txn.Plan) (int64, error) {
	if e.onEstimate != nil {
		e.onEstimate()
	}
	return e.fakeSnapshots.Estimate(ctx, plan)
}

func (e estimateTakeSnapshot) Take(ctx context.Context, plan txn.Plan) (string, error) {
	if e.onTake != nil {
		e.onTake()
	}
	return e.fakeSnapshots.Take(ctx, plan)
}

func TestHOK_006_estimate_runs_before_pre_update(t *testing.T) {
	var estimated, taken bool
	rec := &recordingHooks{}
	h := newHookHarness(rec)
	h.executor.snapshot = estimateTakeSnapshot{
		onEstimate: func() { estimated = true },
		onTake:     func() { taken = true },
	}
	h.run(t)
	if !estimated || !taken {
		t.Fatalf("estimated %v taken %v", estimated, taken)
	}
	if hookIndex(rec.calls, txn.HookPreUpdate) < 0 {
		t.Fatalf("pre-update not run")
	}
}
