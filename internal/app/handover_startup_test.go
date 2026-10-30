package app

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/handover"
	"github.com/KazzyAPI/parachute/internal/state"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type handoverMemoryFiles map[string][]byte

func (m handoverMemoryFiles) ReadFile(path string) ([]byte, error) {
	contents, ok := m[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return contents, nil
}

func (m handoverMemoryFiles) WriteAtomic(path string, data []byte) error {
	m[path] = data
	return nil
}

func (m handoverMemoryFiles) Remove(path string) error {
	delete(m, path)
	return nil
}

type fakeHandoverEngine struct {
	containers []domain.Container
	images     map[string]engine.Image
}

func (f *fakeHandoverEngine) Containers(context.Context) ([]domain.Container, error) {
	return f.containers, nil
}

func (f *fakeHandoverEngine) Inspect(context.Context, string) (engine.Spec, error) {
	return engine.Spec{}, nil
}

func (f *fakeHandoverEngine) Remove(context.Context, string, bool, bool) error { return nil }

func (f *fakeHandoverEngine) Image(_ context.Context, reference string) (engine.Image, bool, error) {
	image, ok := f.images[reference]
	return image, ok, nil
}

type recordingHandoverPublisher struct {
	reports []domain.Report
}

func (r *recordingHandoverPublisher) Publish(_ context.Context, report domain.Report) error {
	r.reports = append(r.reports, report)
	return nil
}

func TestStartupHandoverTakeoverRecordsInterruptedFreeze(t *testing.T) {
	ownID := strings.Repeat("a", 64)
	oldID := strings.Repeat("b", 64)
	marker := handover.Marker{
		Format: handover.Format, Transaction: "txn-20260101T000000Z-abc123", Container: oldID, Digest: "sha256:deadbeef",
	}
	files := handoverMemoryFiles{}
	if err := handover.Write(files, "/s", marker); err != nil {
		t.Fatal(err)
	}
	store := state.NewStore("/s", files, nil)
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	if err := state.NewJournal(store, func() time.Time { return time.Unix(0, 0) }).Begin(context.Background(), txn.OpenRecord{
		ID: marker.Transaction, Host: "local", Stacks: []string{"ops"}, State: txn.StateHandover, SelfUpdate: true, OwnContainer: "parachute",
	}); err != nil {
		t.Fatal(err)
	}
	engine := &fakeHandoverEngine{
		containers: []domain.Container{
			{ID: ownID, Name: "parachute-new", Own: true, Labels: map[string]string{"parachute.transaction": marker.Transaction}, ImageRef: "repo:2"},
		},
		images: map[string]engine.Image{"repo:2": {Digests: []string{marker.Digest}}},
	}
	publisher := &recordingHandoverPublisher{}
	startup := &StartupHandover{
		stateDir: "/s", files: files, engine: engine, host: "local",
		own:   domain.Container{ID: ownID, Name: "parachute-new", Labels: map[string]string{"parachute.transaction": marker.Transaction}, ImageRef: "repo:2"},
		admit: handover.Admission{Role: handover.RoleNew, Marker: marker, HasMarker: true}, takeover: true,
		journal: state.NewJournal(store, func() time.Time { return time.Unix(0, 0) }), reporter: publisher,
		logger: slog.New(slog.DiscardHandler),
	}
	if err := startup.AfterStore(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	document := store.Current()
	if document.Transactions[0].State != string(txn.StateRollbackFail) || document.Transactions[0].Reason != "interrupted" {
		t.Fatalf("transaction %+v", document.Transactions[0])
	}
	if len(document.Freezes) != 1 || len(document.SupersededInstances) != 1 || document.SupersededInstances[0] != oldID {
		t.Fatalf("document freezes=%+v superseded=%v", document.Freezes, document.SupersededInstances)
	}
	if len(publisher.reports) != 1 || publisher.reports[0].Hosts[0].Results[0].State != domain.StateRollbackFailed {
		t.Fatalf("reports %+v", publisher.reports)
	}
}

func TestStartupHandoverSupersededInstanceExits(t *testing.T) {
	ownID := strings.Repeat("c", 64)
	store := state.NewStore("/s", handoverMemoryFiles{}, nil)
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(func(document *state.Document) error {
		document.SupersededInstances = []string{ownID}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	startup := &StartupHandover{own: domain.Container{ID: ownID}}
	err := startup.AfterStore(context.Background(), store)
	if err == nil {
		t.Fatal("expected superseded error")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != domain.ExitConfiguration {
		t.Fatalf("err=%v", err)
	}
}

type fakeSelfFiles struct{ hostname string }

func (fakeSelfFiles) ReadFile(string) ([]byte, error) { return nil, fs.ErrNotExist }

func (fakeSelfFiles) Stat(string) (fs.FileInfo, error) { return nil, nil }

func (f fakeSelfFiles) Hostname() (string, error) { return f.hostname, nil }

func TestStartupHandoverSupersededInstanceExitsWithoutMarker(t *testing.T) {
	ownID := strings.Repeat("c", 64)
	store := state.NewStore("/s", handoverMemoryFiles{}, nil)
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(func(document *state.Document) error {
		document.SupersededInstances = []string{ownID}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	startup := &StartupHandover{
		stateDir: "/s", files: handoverMemoryFiles{}, self: engine.ProbeSelf(fakeSelfFiles{hostname: ownID[:12]}),
		engine: &fakeHandoverEngine{containers: []domain.Container{{ID: ownID, Name: "parachute", State: "running"}}},
	}
	if err := startup.BeforeStore(context.Background()); err != nil {
		t.Fatal(err)
	}
	var exitErr *ExitError
	if err := startup.AfterStore(context.Background(), store); !errors.As(err, &exitErr) || exitErr.Code != domain.ExitConfiguration {
		t.Fatalf("err=%v", err)
	}
}
