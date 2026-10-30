package selfupdate

import (
	"context"
	"io"
	"io/fs"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/graph"
	"github.com/KazzyAPI/parachute/internal/handover"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/registry"
	"github.com/KazzyAPI/parachute/internal/txn"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
)

type memFS struct {
	files map[string][]byte
	lock  sync.Mutex
}

func newMemFS() *memFS { return &memFS{files: map[string][]byte{}} }

func (m *memFS) ReadFile(path string) ([]byte, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	data, ok := m.files[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return data, nil
}

func (m *memFS) WriteAtomic(path string, data []byte) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.files[path] = append([]byte(nil), data...)
	return nil
}

func (m *memFS) Remove(path string) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.files, path)
	return nil
}

type fakeEngine struct {
	spec        engine.Spec
	containers  []domain.Container
	renamed     string
	removed     []string
	pulls       int
	replacement string
}

func (f *fakeEngine) Inspect(_ context.Context, id string) (engine.Spec, error) {
	if id == f.replacement {
		return engine.Spec{ID: id, Name: f.spec.Name, ImageID: "img-new"}, nil
	}
	return f.spec, nil
}
func (f *fakeEngine) Stop(context.Context, string, string, time.Duration) error { return nil }
func (f *fakeEngine) Start(context.Context, string) error                       { return nil }
func (f *fakeEngine) Rename(_ context.Context, _, name string) error {
	f.renamed = name
	return nil
}
func (f *fakeEngine) Remove(_ context.Context, id string, _, _ bool) error {
	f.removed = append(f.removed, id)
	return nil
}
func (f *fakeEngine) RemoveImage(context.Context, string) error { return nil }
func (f *fakeEngine) Image(context.Context, string) (engine.Image, bool, error) {
	return engine.Image{ID: "sha256:local"}, true, nil
}
func (f *fakeEngine) Pull(context.Context, string, string, engine.RegistryAuth) error {
	f.pulls++
	return nil
}
func (f *fakeEngine) Tag(context.Context, string, string) error { return nil }
func (f *fakeEngine) Events(context.Context, time.Time, map[string][]string) (<-chan engine.Event, <-chan error) {
	return nil, nil
}
func (f *fakeEngine) Containers(context.Context) ([]domain.Container, error) {
	return f.containers, nil
}
func (f *fakeEngine) Runtime(context.Context, string) (engine.Runtime, error) {
	return engine.Runtime{Status: "running", Running: true}, nil
}

type recordingJournal struct {
	record  txn.OpenRecord
	states  []txn.State
	outcome txn.Outcome
}

func (j *recordingJournal) Begin(_ context.Context, record txn.OpenRecord) error {
	j.record = record
	return nil
}
func (j *recordingJournal) Advance(_ context.Context, _ string, state txn.State) error {
	j.states = append(j.states, state)
	return nil
}
func (j *recordingJournal) Finish(_ context.Context, _ string, outcome txn.Outcome) error {
	j.outcome = outcome
	return nil
}
func (j *recordingJournal) Open(context.Context) ([]txn.OpenRecord, error) { return nil, nil }

type fakeSnapshotter struct{}

func (fakeSnapshotter) Estimate(context.Context, txn.Plan) (int64, error) { return 0, nil }
func (fakeSnapshotter) Take(context.Context, txn.Plan) (string, error)    { return "none", nil }
func (fakeSnapshotter) Validate(context.Context, string) error            { return nil }
func (fakeSnapshotter) Restore(context.Context, string) error             { return nil }
func (fakeSnapshotter) Discard(context.Context, string) error             { return nil }
func (fakeSnapshotter) Retain(context.Context, string, string, int) error { return nil }

type fakeRecreator struct {
	replacement string
}

func (f *fakeRecreator) Reproducible(context.Context, string) error { return nil }
func (f *fakeRecreator) Replacement(context.Context, txn.Plan, string, string) (string, error) {
	if f.replacement == "" {
		f.replacement = "replacement-id"
	}
	return f.replacement, nil
}

type fakeVerifier struct {
	passed bool
}

func (f fakeVerifier) Verify(context.Context, txn.Plan, string) (txn.Verdict, error) {
	if f.passed {
		return txn.Verdict{Passed: true}, nil
	}
	return txn.Verdict{Passed: false, Reason: "verify_failed", Check: "stable", Output: "unhealthy"}, nil
}
func (f fakeVerifier) VerifyBase(context.Context, txn.Plan, string) (txn.Verdict, error) {
	return f.Verify(context.Background(), txn.Plan{}, "")
}

type fakeClock struct{}

func (fakeClock) Now() time.Time { return time.Unix(0, 0) }

const testContainerID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func ownCandidate() *pipeline.Candidate {
	container := domain.Container{
		ID: testContainerID, Name: "parachute", State: "running", ImageRef: "repo/p:1", Own: true,
		Labels: map[string]string{"com.docker.compose.project": "ops", "com.docker.compose.service": "parachute"},
	}
	result := domain.NewResult("local", container).Available("sha256:new")
	result.TargetImage = "repo/p:1"
	return &pipeline.Candidate{Container: container, Result: result}
}

func TestRunnerHandoverCommitsAndDefersMarkerRemoval(t *testing.T) {
	eng := &fakeEngine{spec: engine.Spec{ID: testContainerID, Name: "parachute"}}
	journal := &recordingJournal{}
	files := newMemFS()
	runner := NewRunner(
		eng, journal, fakeSnapshotter{}, &fakeRecreator{}, fakeVerifier{passed: true}, files, "/state",
		txn.NewIDGenerator(fakeClock{}.Now), fakeClock{}, slog.New(slog.NewTextHandler(io.Discard, nil)),
		graphOptions(), func(registry.Reference) engine.RegistryAuth { return "" },
	)
	ticket, err := runner.Handover(context.Background(), "local", ownCandidate())
	if err != nil {
		t.Fatal(err)
	}
	if !ticket.RemoveMarker {
		t.Fatalf("ticket %+v", ticket)
	}
	if journal.outcome.State != txn.StateCommitted || journal.outcome.Superseded != testContainerID {
		t.Fatalf("outcome %+v", journal.outcome)
	}
	if _, ok, err := handover.Read(files, "/state"); !ok || err != nil {
		t.Fatalf("marker before finish ok=%v err=%v", ok, err)
	}
	if err := runner.Finish(context.Background(), ticket); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := handover.Read(files, "/state"); ok || err != nil {
		t.Fatalf("marker after finish ok=%v err=%v", ok, err)
	}
}

func TestRunnerHandoverRejectsPublishedPort(t *testing.T) {
	spec := engine.Spec{ID: testContainerID, Name: "parachute", HostConfig: containerHostConfigWithPort()}
	eng := &fakeEngine{spec: spec}
	journal := &recordingJournal{}
	runner := NewRunner(
		eng, journal, fakeSnapshotter{}, &fakeRecreator{}, fakeVerifier{passed: true}, newMemFS(), "/state",
		txn.NewIDGenerator(fakeClock{}.Now), fakeClock{}, slog.New(slog.NewTextHandler(io.Discard, nil)),
		graphOptions(), func(registry.Reference) engine.RegistryAuth { return "" },
	)
	candidate := ownCandidate()
	if _, err := runner.Handover(context.Background(), "local", candidate); err != nil {
		t.Fatal(err)
	}
	if candidate.Result.State != domain.StateFailed || candidate.Result.Reason != "config_unreproducible" {
		t.Fatalf("result %+v", candidate.Result)
	}
	if journal.outcome.State != txn.StateAborted {
		t.Fatalf("outcome %+v", journal.outcome)
	}
}

func TestRunnerHandoverVerifyFailureRollsBack(t *testing.T) {
	eng := &fakeEngine{spec: engine.Spec{ID: testContainerID, Name: "parachute"}}
	journal := &recordingJournal{}
	files := newMemFS()
	runner := NewRunner(
		eng, journal, fakeSnapshotter{}, &fakeRecreator{}, fakeVerifier{passed: false}, files, "/state",
		txn.NewIDGenerator(fakeClock{}.Now), fakeClock{}, slog.New(slog.NewTextHandler(io.Discard, nil)),
		graphOptions(), func(registry.Reference) engine.RegistryAuth { return "" },
	)
	candidate := ownCandidate()
	if _, err := runner.Handover(context.Background(), "local", candidate); err != nil {
		t.Fatal(err)
	}
	if candidate.Result.State != domain.StateRolledBack || candidate.Result.Reason != "verify_failed" {
		t.Fatalf("result %+v", candidate.Result)
	}
	if journal.outcome.State != txn.StateRolledBack {
		t.Fatalf("outcome %+v", journal.outcome)
	}
	if _, ok, _ := handover.Read(files, "/state"); ok {
		t.Fatal("marker should be removed on rollback")
	}
}

func graphOptions() graph.Options { return graph.Options{TransactionTimeout: time.Minute} }

func containerHostConfigWithPort() *container.HostConfig {
	return &container.HostConfig{PortBindings: map[network.Port][]network.PortBinding{
		network.MustParsePort("8080/tcp"): {{HostPort: "8080"}},
	}}
}
