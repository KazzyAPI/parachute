package app

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/KazzyAPI/parachute/internal/backup"
	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type rollbackFakeEngine struct {
	running map[string]bool
	started []string
}

func (f *rollbackFakeEngine) Containers(context.Context) ([]domain.Container, error) {
	return nil, nil
}

func (f *rollbackFakeEngine) Runtime(_ context.Context, id string) (engine.Runtime, error) {
	return engine.Runtime{Running: f.running[id]}, nil
}

func (f *rollbackFakeEngine) Inspect(_ context.Context, id string) (engine.Spec, error) {
	return engine.Spec{Name: id}, nil
}

func (f *rollbackFakeEngine) Remove(context.Context, string, bool, bool) error { return nil }

func (f *rollbackFakeEngine) Rename(context.Context, string, string) error { return nil }

func (f *rollbackFakeEngine) Start(_ context.Context, id string) error {
	f.started = append(f.started, id)
	return nil
}

type rollbackFakeSnapshots struct{}

func (rollbackFakeSnapshots) ValidateTransaction(context.Context, string, string, string, string) error {
	return nil
}

func (rollbackFakeSnapshots) Restore(context.Context, string) error { return nil }

func (rollbackFakeSnapshots) SkippedFromSnapshot(context.Context, string) ([]backup.SkippedRecord, error) {
	return nil, nil
}

type rollbackFakeVerifier struct{ verified []string }

func (f *rollbackFakeVerifier) Verify(context.Context, txn.Plan, string) (txn.Verdict, error) {
	return txn.Verdict{Passed: true}, nil
}

func (f *rollbackFakeVerifier) VerifyBase(_ context.Context, _ txn.Plan, id string) (txn.Verdict, error) {
	f.verified = append(f.verified, id)
	return txn.Verdict{Passed: false, Check: "stable"}, nil
}

type rollbackFakeLabels struct{}

func (rollbackFakeLabels) Value(domain.Container, string) (string, bool) { return "", false }

func TestRollbackVerifiesOnlyRestartedOriginals(t *testing.T) {
	fake := &rollbackFakeEngine{running: map[string]bool{"untouched": true}}
	verifier := &rollbackFakeVerifier{}
	finisher := NewRollbackFinisher(fake, rollbackFakeSnapshots{}, verifier, rollbackFakeLabels{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	plan := txn.Plan{ID: "txn-20261030T000000Z-abcdef", Snapshot: "none"}
	originals := []txn.Original{{ID: "untouched", Running: true}, {ID: "stopped", Running: true}}
	outcome, err := finisher.FinishRollback(context.Background(), plan, originals, nil, domain.StateFailed, "transaction_timeout", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(verifier.verified) != 1 || verifier.verified[0] != "stopped" {
		t.Fatalf("verified %v, want only the restarted original", verifier.verified)
	}
	if outcome.State != txn.StateRollbackFail {
		t.Fatalf("restarted failing original must fail rollback, got %s", outcome.State)
	}
}
