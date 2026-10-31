package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/KazzyAPI/parachute/internal/backup"
	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type rollbackFakeEngine struct {
	running    map[string]bool
	started    []string
	containers []domain.Container
	runtimeErr error
}

func (f *rollbackFakeEngine) Containers(context.Context) ([]domain.Container, error) {
	return f.containers, nil
}

func (f *rollbackFakeEngine) Runtime(_ context.Context, id string) (engine.Runtime, error) {
	return engine.Runtime{Running: f.running[id]}, f.runtimeErr
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

type rollbackFakeSnapshots struct{ restored *bool }

func (rollbackFakeSnapshots) ValidateTransaction(context.Context, string, string, string, string) error {
	return nil
}

func (rollbackFakeSnapshots) BackupMode(domain.Container) string { return "none" }

func (f rollbackFakeSnapshots) Restore(context.Context, string) error {
	if f.restored != nil {
		*f.restored = true
	}
	return nil
}

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

func TestRollbackVerifiesOnlyRestartedOriginals(t *testing.T) {
	fake := &rollbackFakeEngine{running: map[string]bool{"untouched": true}}
	verifier := &rollbackFakeVerifier{}
	finisher := NewRollbackFinisher(fake, rollbackFakeSnapshots{}, verifier, slog.New(slog.NewTextHandler(io.Discard, nil)))
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

func TestRollbackRestoresDataWhenRuntimeLookupFails(t *testing.T) {
	const transaction = "txn-20261030T000000Z-abcdef"
	fake := &rollbackFakeEngine{
		runtimeErr: errors.New("engine unavailable"),
		containers: []domain.Container{{ID: "repl", Name: "web", Labels: map[string]string{"parachute.transaction": transaction}}},
	}
	var restored bool
	finisher := NewRollbackFinisher(fake, rollbackFakeSnapshots{restored: &restored}, &rollbackFakeVerifier{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := finisher.FinishRollback(context.Background(), txn.Plan{ID: transaction, Host: "local", Stacks: []string{"s"}, Snapshot: "snap-1"}, nil, nil, domain.StateFailed, "verify_failed", ""); err != nil {
		t.Fatal(err)
	}
	if !restored {
		t.Fatal("data must be restored when the replacement's runtime cannot be read")
	}
}

type modeSnapshots struct{ rollbackFakeSnapshots }

func (modeSnapshots) BackupMode(domain.Container) string { return "none" }

func TestRollbackReportsBackupNoneFromEffectiveMode(t *testing.T) {
	fake := &rollbackFakeEngine{}
	finisher := NewRollbackFinisher(fake, modeSnapshots{}, &rollbackFakeVerifier{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	web := domain.Container{ID: "web-id", Name: "web"}
	candidate := &pipeline.Candidate{Container: web, Result: domain.NewResult("local", web)}
	plan := txn.Plan{ID: "txn-20261030T000000Z-abcdef", Snapshot: "none"}
	if _, err := finisher.FinishRollback(context.Background(), plan, nil, []*pipeline.Candidate{candidate}, domain.StateFailed, "verify_failed", ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(candidate.Result.Detail, "data not restored: backup=none") {
		t.Fatalf("detail %q", candidate.Result.Detail)
	}
}
