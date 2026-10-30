package graph

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/recreate"
	"github.com/KazzyAPI/parachute/internal/registry"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type fakeEngine struct {
	stops, removes, pulls int
	started               []string
	local                 engine.Image
}

func (f *fakeEngine) Inspect(context.Context, string) (engine.Spec, error) { return engine.Spec{}, nil }
func (f *fakeEngine) Stop(context.Context, string, string, time.Duration) error {
	f.stops++
	return nil
}
func (f *fakeEngine) Start(_ context.Context, id string) error {
	f.started = append(f.started, id)
	return nil
}
func (f *fakeEngine) Rename(context.Context, string, string) error { return nil }
func (f *fakeEngine) Remove(context.Context, string, bool, bool) error {
	f.removes++
	return nil
}
func (f *fakeEngine) RemoveImage(context.Context, string) error { return nil }
func (f *fakeEngine) Image(context.Context, string) (engine.Image, bool, error) {
	return f.local, f.local.ID != "", nil
}
func (f *fakeEngine) Pull(context.Context, string, string, engine.RegistryAuth) error {
	f.pulls++
	return nil
}
func (f *fakeEngine) Tag(context.Context, string, string) error { return nil }
func (f *fakeEngine) Events(context.Context, time.Time, map[string][]string) (<-chan engine.Event, <-chan error) {
	return nil, nil
}

type fakeJournal struct {
	states  []txn.State
	outcome txn.Outcome
}

func (f *fakeJournal) Begin(context.Context, txn.OpenRecord) error { return nil }
func (f *fakeJournal) Advance(_ context.Context, _ string, state txn.State) error {
	f.states = append(f.states, state)
	return nil
}
func (f *fakeJournal) Finish(_ context.Context, _ string, outcome txn.Outcome) error {
	f.outcome = outcome
	return nil
}
func (f *fakeJournal) Open(context.Context) ([]txn.OpenRecord, error) { return nil, nil }

type fakeSnapshots struct {
	err     error
	skipped map[string][]string
}

func (f fakeSnapshots) Estimate(context.Context, txn.Plan) (int64, error) { return 0, nil }
func (f fakeSnapshots) Take(context.Context, txn.Plan) (string, error)    { return "snap-1", f.err }
func (f fakeSnapshots) Validate(context.Context, string) error            { return nil }
func (f fakeSnapshots) Restore(context.Context, string) error             { return nil }
func (f fakeSnapshots) Discard(context.Context, string) error             { return nil }
func (f fakeSnapshots) Retain(context.Context, string, string, int) error { return nil }
func (f fakeSnapshots) SkippedDestinations(context.Context, string) (map[string][]string, error) {
	return f.skipped, nil
}

type fakeRecreator struct{ unreproducible error }

func (f fakeRecreator) Reproducible(context.Context, string) error { return f.unreproducible }
func (f fakeRecreator) Replacement(context.Context, txn.Plan, string, string) (string, error) {
	return "replacement", nil
}

type fakeVerifier struct{ verdict txn.Verdict }

func (f fakeVerifier) Verify(context.Context, txn.Plan, string) (txn.Verdict, error) {
	return f.verdict, nil
}

func (f fakeVerifier) VerifyBase(context.Context, txn.Plan, string) (txn.Verdict, error) {
	return f.verdict, nil
}

type fakeRollbacker struct {
	called    bool
	final     domain.State
	reason    string
	detail    string
	originals []txn.Original
}

func (f *fakeRollbacker) FinishRollback(_ context.Context, _ txn.Plan, originals []txn.Original, candidates []*pipeline.Candidate, final domain.State, reason, detail string) (txn.Outcome, error) {
	f.called, f.final, f.reason, f.detail, f.originals = true, final, reason, detail, originals
	for _, candidate := range candidates {
		candidate.Result = candidate.Result.Conclude(final, reason, detail)
	}
	return txn.Outcome{State: txn.StateRolledBack, Result: final, Reason: txn.Reason(reason)}, nil
}

type fakeClock struct{}

func (fakeClock) Now() time.Time { return time.Unix(0, 0) }

type harness struct {
	engine     *fakeEngine
	journal    *fakeJournal
	rollbacker *fakeRollbacker
	executor   *Executor
	candidates []*pipeline.Candidate
	containers []domain.Container
}

func newHarness(snapshots fakeSnapshots, recreator fakeRecreator, verifier fakeVerifier) *harness {
	h := &harness{engine: &fakeEngine{}, journal: &fakeJournal{}, rollbacker: &fakeRollbacker{}}
	web := domain.Container{ID: "web-id", Name: "web", State: "running", ImageRef: "repo/web:1"}
	h.containers = []domain.Container{web}
	result := domain.NewResult("local", web).Available("sha256:new")
	h.candidates = []*pipeline.Candidate{{Container: web, Result: result}}
	h.executor = NewExecutor(NewBuilder(), h.engine, h.journal, snapshots, recreator, verifier, h.rollbacker,
		txn.NewIDGenerator(fakeClock{}.Now), fakeClock{}, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{}, func(registry.Reference) engine.RegistryAuth { return "" })
	return h
}

func (h *harness) run(t *testing.T) {
	t.Helper()
	if err := h.executor.Run(context.Background(), "local", h.containers, h.candidates, ""); err != nil {
		t.Fatal(err)
	}
}

func TestVER_008_failed_verification_rolls_back_with_reason_and_detail(t *testing.T) {
	h := newHarness(fakeSnapshots{}, fakeRecreator{}, fakeVerifier{txn.Verdict{Reason: "verify_failed", Check: "health", Output: "db connection refused"}})
	h.run(t)
	if !h.rollbacker.called || h.rollbacker.final != domain.StateRolledBack || h.rollbacker.reason != "verify_failed" {
		t.Fatalf("rollback %+v", h.rollbacker)
	}
	if h.rollbacker.detail != "health: db connection refused" {
		t.Fatalf("detail %q", h.rollbacker.detail)
	}
	if len(h.rollbacker.originals) != 1 || !h.rollbacker.originals[0].Running || !h.rollbacker.originals[0].Updated {
		t.Fatalf("originals %+v", h.rollbacker.originals)
	}
	if h.journal.states[len(h.journal.states)-1] != txn.StateRollingBack || h.journal.outcome.State != txn.StateRolledBack {
		t.Fatalf("journal %v %+v", h.journal.states, h.journal.outcome)
	}
	if h.engine.removes != 0 {
		t.Fatalf("originals removed before commit")
	}
}

func TestVER_011_timeout_detail_names_deadline_and_label(t *testing.T) {
	verdict := txn.Verdict{Reason: "verify_timeout", Check: "health", Deadline: 120 * time.Second}
	h := newHarness(fakeSnapshots{}, fakeRecreator{}, fakeVerifier{verdict})
	h.run(t)
	for _, part := range []string{"120s", "parachute.verify.timeout"} {
		if !contains(h.rollbacker.detail, part) {
			t.Fatalf("detail %q missing %q", h.rollbacker.detail, part)
		}
	}
}

func TestUPD_009_commit_removes_originals_and_marks_updated(t *testing.T) {
	h := newHarness(fakeSnapshots{}, fakeRecreator{}, fakeVerifier{txn.Verdict{Passed: true}})
	h.run(t)
	if h.rollbacker.called || h.journal.outcome.State != txn.StateCommitted || h.engine.removes != 1 {
		t.Fatalf("rollback %v outcome %+v removes %d", h.rollbacker.called, h.journal.outcome, h.engine.removes)
	}
	if h.candidates[0].Result.State != domain.StateUpdated {
		t.Fatalf("state %s", h.candidates[0].Result.State)
	}
}

func TestUPD_014_snapshot_failure_marks_failed_and_aborts(t *testing.T) {
	h := newHarness(fakeSnapshots{err: errors.New("insufficient_space: estimated 1 free 0")}, fakeRecreator{}, fakeVerifier{})
	h.run(t)
	result := h.candidates[0].Result
	if result.State != domain.StateFailed || result.Reason != "insufficient_space" || result.Transaction == "" {
		t.Fatalf("result %+v", result)
	}
	if h.journal.outcome.State != txn.StateAborted || h.rollbacker.called {
		t.Fatalf("journal %+v rollback %v", h.journal.outcome, h.rollbacker.called)
	}
}

func TestUPD_014_unreproducible_configuration_changes_nothing(t *testing.T) {
	failure := recreate.Failure{State: "failed", Reason: "config_unreproducible", Detail: "AutoRemove"}
	h := newHarness(fakeSnapshots{}, fakeRecreator{unreproducible: failure}, fakeVerifier{})
	h.run(t)
	result := h.candidates[0].Result
	if result.State != domain.StateFailed || result.Reason != "config_unreproducible" || h.engine.stops != 0 {
		t.Fatalf("result %+v stops %d", result, h.engine.stops)
	}
}

func contains(text, part string) bool {
	for i := 0; i+len(part) <= len(text); i++ {
		if text[i:i+len(part)] == part {
			return true
		}
	}
	return false
}

func TestDET_004_local_image_is_not_pulled(t *testing.T) {
	h := newHarness(fakeSnapshots{}, fakeRecreator{}, fakeVerifier{txn.Verdict{Passed: true}})
	h.engine.local = engine.Image{ID: "sha256:new"}
	h.candidates[0].Result = domain.NewResult("local", h.containers[0]).Available("sha256:new")
	h.run(t)
	if h.engine.pulls != 0 {
		t.Fatalf("pulls %d", h.engine.pulls)
	}
}

func TestUPD_020_registry_digest_is_pulled(t *testing.T) {
	h := newHarness(fakeSnapshots{}, fakeRecreator{}, fakeVerifier{txn.Verdict{Passed: true}})
	h.engine.local = engine.Image{ID: "sha256:old"}
	h.run(t)
	if h.engine.pulls != 1 {
		t.Fatalf("pulls %d", h.engine.pulls)
	}
}

func TestDEP_003_restarted_dependent_shares_transaction(t *testing.T) {
	h := newHarness(fakeSnapshots{}, fakeRecreator{}, fakeVerifier{txn.Verdict{Passed: true}})
	app := domain.Container{ID: "app-id", Name: "app", State: "running", ImageRef: "repo/app:1", Labels: map[string]string{"parachute.depends-on": "web"}}
	h.containers = append(h.containers, app)
	h.candidates = append(h.candidates, &pipeline.Candidate{Container: app, Result: domain.NewResult("local", app)})
	h.run(t)
	web, dependent := h.candidates[0].Result, h.candidates[1].Result
	if web.Transaction == "" || dependent.Transaction != web.Transaction {
		t.Fatalf("web %q dependent %q", web.Transaction, dependent.Transaction)
	}
	if dependent.State == domain.StateUpdated {
		t.Fatalf("dependent marked updated")
	}
}

func stoppedHarness(options Options) *harness {
	h := newHarness(fakeSnapshots{}, fakeRecreator{}, fakeVerifier{txn.Verdict{Passed: true}})
	h.containers[0].State = "exited"
	h.candidates[0].Container.State = "exited"
	h.executor.options = options
	return h
}

func TestSEL_010_stopped_container_replacement_is_not_started(t *testing.T) {
	h := stoppedHarness(Options{})
	h.run(t)
	if len(h.engine.started) != 0 {
		t.Fatalf("started %v", h.engine.started)
	}
	if h.candidates[0].Result.State != domain.StateUpdated {
		t.Fatalf("state %s", h.candidates[0].Result.State)
	}
}

func TestSEL_010_revive_stopped_starts_replacement(t *testing.T) {
	h := stoppedHarness(Options{ReviveStopped: true})
	h.run(t)
	if len(h.engine.started) != 1 || h.engine.started[0] != "replacement" {
		t.Fatalf("started %v", h.engine.started)
	}
}

func TestUPD_012_no_restart_does_not_start_replacement(t *testing.T) {
	h := newHarness(fakeSnapshots{}, fakeRecreator{}, fakeVerifier{txn.Verdict{Passed: true}})
	h.executor.options = Options{NoRestart: true}
	h.run(t)
	if len(h.engine.started) != 0 {
		t.Fatalf("started %v", h.engine.started)
	}
}

type retainCounter struct {
	fakeSnapshots
	stacks *[]string
}

func (r retainCounter) Retain(_ context.Context, _, stack string, _ int) error {
	*r.stacks = append(*r.stacks, stack)
	return nil
}

func TestBKP_008_retention_runs_after_completed_rollback(t *testing.T) {
	var retained []string
	verdict := txn.Verdict{Reason: "verify_failed", Check: "health"}
	h := newHarness(fakeSnapshots{}, fakeRecreator{}, fakeVerifier{verdict})
	h.executor.snapshot = retainCounter{stacks: &retained}
	h.run(t)
	if !h.rollbacker.called || len(retained) != 1 {
		t.Fatalf("rollback %v retained %v", h.rollbacker.called, retained)
	}
}

func TestBKP_012_commit_detail_names_skipped_system_mounts(t *testing.T) {
	h := newHarness(fakeSnapshots{skipped: map[string][]string{"web": {"/host-etc", "/host-proc"}}}, fakeRecreator{}, fakeVerifier{txn.Verdict{Passed: true}})
	h.run(t)
	want := "skipped system mount /host-etc; skipped system mount /host-proc"
	if got := h.candidates[0].Result.Detail; got != want {
		t.Fatalf("detail %q, want %q", got, want)
	}
}
