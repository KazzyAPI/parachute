package graph

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
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
	calls                 []string
	local                 engine.Image
	health                []string
}

func (f *fakeEngine) Inspect(context.Context, string) (engine.Spec, error) {
	if len(f.health) == 0 {
		return engine.Spec{}, nil
	}
	status := f.health[0]
	f.health = f.health[1:]
	return engine.Spec{HealthStatus: status}, nil
}
func (f *fakeEngine) Stop(_ context.Context, id string, _ string, _ time.Duration) error {
	f.stops++
	f.calls = append(f.calls, "stop "+id)
	return nil
}
func (f *fakeEngine) Start(_ context.Context, id string) error {
	f.started = append(f.started, id)
	f.calls = append(f.calls, "start "+id)
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

type fakeSnapshots struct{ err error }

func (f fakeSnapshots) Estimate(context.Context, txn.Plan) (int64, error) { return 0, nil }
func (f fakeSnapshots) Take(context.Context, txn.Plan) (string, error)    { return "snap-1", f.err }
func (f fakeSnapshots) Validate(context.Context, string) error            { return nil }
func (f fakeSnapshots) Restore(context.Context, string) error             { return nil }
func (f fakeSnapshots) Discard(context.Context, string) error             { return nil }
func (f fakeSnapshots) Retain(context.Context, string, string, int) error { return nil }

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
	h.executor = NewExecutor(NewBuilder(), h.engine, h.journal, snapshots, recreator, verifier, h.rollbacker, txn.NoHooks{}, NoSelfRunner{},
		txn.NewIDGenerator(fakeClock{}.Now), fakeClock{}, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{}, func(registry.Reference) engine.RegistryAuth { return "" })
	return h
}

func (h *harness) run(t *testing.T) {
	t.Helper()
	if _, err := h.executor.Run(context.Background(), "local", h.containers, h.candidates, ""); err != nil {
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

func TestDEP_005_service_healthy_dependent_starts_after_dependency_is_healthy(t *testing.T) {
	h := newHarness(fakeSnapshots{}, fakeRecreator{}, fakeVerifier{txn.Verdict{Passed: true}})
	db := container("db", "shop", "db", nil, nil, "")
	db.ID, db.ImageRef = "db-id", "repo/db:1"
	app := container("app", "shop", "app", map[string]string{"com.docker.compose.depends_on": "db:service_healthy:true"}, nil, "")
	app.ID = "app-id"
	h.containers = []domain.Container{app, db}
	h.candidates = []*pipeline.Candidate{
		{Container: db, Result: domain.NewResult("local", db).Available("sha256:new")},
		{Container: app, Result: domain.NewResult("local", app)},
	}
	h.engine.health = []string{"starting", "healthy"}
	h.run(t)
	if got := h.engine.started; len(got) != 2 || got[0] != "replacement" || got[1] != "app-id" || len(h.engine.health) != 0 {
		t.Fatalf("started %v, unread health %v", got, h.engine.health)
	}
}

func TestDEP_005_start_order_follows_dependencies_across_updated_and_restarted(t *testing.T) {
	h := newHarness(fakeSnapshots{}, fakeRecreator{}, fakeVerifier{txn.Verdict{Passed: true}})
	a := container("a", "", "", nil, nil, "")
	b := container("b", "", "", map[string]string{"parachute.depends-on": "a"}, nil, "")
	c := container("c", "", "", map[string]string{"parachute.depends-on": "b"}, nil, "")
	a.ID, b.ID, c.ID = "a-id", "b-id", "c-id"
	a.ImageRef, c.ImageRef = "repo/a:1", "repo/c:1"
	h.containers = []domain.Container{a, b, c}
	h.candidates = []*pipeline.Candidate{
		{Container: a, Result: domain.NewResult("local", a).Available("sha256:new")},
		{Container: b, Result: domain.NewResult("local", b)},
		{Container: c, Result: domain.NewResult("local", c).Available("sha256:new")},
	}
	h.run(t)
	if got := h.engine.started; len(got) != 3 || got[0] != "replacement" || got[1] != "b-id" || got[2] != "replacement" {
		t.Fatalf("started %v, want a, b, c", got)
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

type steppingClock struct{ now time.Time }

func (c *steppingClock) Now() time.Time { return c.now }

type slowVerifier struct {
	clock  *steppingClock
	step   time.Duration
	budget time.Duration
}

func (v *slowVerifier) Verify(ctx context.Context, _ txn.Plan, _ string) (txn.Verdict, error) {
	if deadline, ok := ctx.Deadline(); ok {
		v.budget = time.Until(deadline)
	}
	v.clock.now = v.clock.now.Add(v.step)
	return txn.Verdict{Passed: true}, nil
}

func (v *slowVerifier) VerifyBase(context.Context, txn.Plan, string) (txn.Verdict, error) {
	return txn.Verdict{Passed: true}, nil
}

func TestUPD_019_ceiling_hit_during_verification_rolls_back_within_bound(t *testing.T) {
	clock := &steppingClock{now: time.Unix(0, 0)}
	verifier := &slowVerifier{clock: clock, step: 25 * time.Second}
	h := newHarness(fakeSnapshots{}, fakeRecreator{}, fakeVerifier{})
	h.executor.clock = clock
	h.executor.verifier = verifier
	h.executor.options = Options{TransactionTimeout: 20 * time.Second}
	h.run(t)
	if !h.rollbacker.called || h.rollbacker.reason != "transaction_timeout" || h.rollbacker.final != domain.StateRolledBack {
		t.Fatalf("rollback %+v", h.rollbacker)
	}
	if verifier.budget <= 0 || verifier.budget > 20*time.Second {
		t.Fatalf("verify budget %s exceeds ceiling", verifier.budget)
	}
}

type expiringSnapshots struct {
	fakeSnapshots
	clock *steppingClock
}

func (s expiringSnapshots) Take(context.Context, txn.Plan) (string, error) {
	s.clock.now = s.clock.now.Add(time.Minute)
	return "snap-1", nil
}

func TestUPD_019_ceiling_before_update_phase_aborts_with_failed(t *testing.T) {
	clock := &steppingClock{now: time.Unix(0, 0)}
	h := newHarness(fakeSnapshots{}, fakeRecreator{}, fakeVerifier{})
	h.executor.clock = clock
	h.executor.snapshot = expiringSnapshots{clock: clock}
	h.executor.options = Options{TransactionTimeout: 20 * time.Second}
	h.run(t)
	if h.rollbacker.reason != "transaction_timeout" || h.rollbacker.final != domain.StateFailed || h.journal.outcome.State != txn.StateAborted || h.engine.stops != 0 {
		t.Fatalf("rollback %+v journal %+v stops %d", h.rollbacker, h.journal.outcome, h.engine.stops)
	}
}

func TestUPD_013_rolling_restart_stops_and_starts_each_replica_in_turn(t *testing.T) {
	h := newHarness(fakeSnapshots{}, fakeRecreator{}, fakeVerifier{txn.Verdict{Passed: true}})
	labels := map[string]string{"com.docker.compose.project": "shop", "com.docker.compose.service": "worker"}
	h.containers = nil
	h.candidates = nil
	for _, name := range []string{"a", "b", "c"} {
		container := domain.Container{ID: name + "-id", Name: name, State: "running", ImageRef: "repo/w:1", Labels: labels}
		h.containers = append(h.containers, container)
		h.candidates = append(h.candidates, &pipeline.Candidate{Container: container, Result: domain.NewResult("local", container).Available("sha256:new")})
	}
	h.executor.options.Rolling = true
	h.run(t)
	want := []string{"stop a-id", "start replacement", "stop b-id", "start replacement", "stop c-id", "start replacement"}
	if !slices.Equal(h.engine.calls, want) {
		t.Fatalf("calls %v want %v", h.engine.calls, want)
	}
}
