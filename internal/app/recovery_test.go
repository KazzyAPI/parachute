package app

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/state"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type recordingJournal struct {
	open     []txn.OpenRecord
	advanced []txn.State
	outcome  txn.Outcome
	finished bool
}

func (j *recordingJournal) Open(context.Context) ([]txn.OpenRecord, error) { return j.open, nil }
func (j *recordingJournal) Advance(_ context.Context, _ string, next txn.State) error {
	j.advanced = append(j.advanced, next)
	return nil
}
func (j *recordingJournal) Finish(_ context.Context, _ string, outcome txn.Outcome) error {
	j.outcome, j.finished = outcome, true
	return nil
}
func (j *recordingJournal) Snapshots(string, string) []state.SnapshotRecord {
	return []state.SnapshotRecord{{ID: "snap-20260101T000000Z-aaaaaa", Transaction: "txn-20260101T000000Z-abc123"}}
}

type staticLister []domain.Container

func (l staticLister) Containers(context.Context) ([]domain.Container, error) { return l, nil }

type recordingFinisher struct {
	final  domain.State
	reason string
	plan   txn.Plan
}

func (f *recordingFinisher) FinishRollback(_ context.Context, plan txn.Plan, _ []txn.Original, candidates []*pipeline.Candidate, final domain.State, reason, _ string) (txn.Outcome, error) {
	f.plan, f.final, f.reason = plan, final, reason
	for _, candidate := range candidates {
		candidate.Result = candidate.Result.Conclude(final, reason, "")
		candidate.Result.Transaction = plan.ID
	}
	return txn.Outcome{State: txn.StateRolledBack, Result: final, Reason: txn.Reason(reason)}, nil
}

func recoverWith(record txn.OpenRecord) (*recordingJournal, *recordingFinisher, *Recovery) {
	journal := &recordingJournal{open: []txn.OpenRecord{record}}
	finisher := &recordingFinisher{}
	lister := staticLister{{ID: "orig", Name: "web-parachute-abc123", Labels: map[string]string{}}}
	hosts := map[string]RecoveryHost{"local": {Lister: lister, Finisher: finisher}}
	recovery := NewRecovery(hosts, journal, slog.New(slog.NewTextHandler(io.Discard, nil)))
	recovery.Recover(context.Background())
	return journal, finisher, recovery
}

func record(openState txn.State) txn.OpenRecord {
	return txn.OpenRecord{
		ID: "txn-20260101T000000Z-abc123", Host: "local", Stacks: []string{"web"}, State: openState,
		Originals: []txn.Original{{ID: "orig", Running: true, Updated: true}},
	}
}

func TestUPD_016_verify_phase_is_rolled_back_as_interrupted(t *testing.T) {
	journal, finisher, recovery := recoverWith(record(txn.StateVerify))
	if finisher.final != domain.StateRolledBack || finisher.reason != "interrupted" || finisher.plan.Snapshot != "snap-20260101T000000Z-aaaaaa" {
		t.Fatalf("finisher %+v", finisher)
	}
	if len(journal.advanced) != 1 || journal.advanced[0] != txn.StateRollingBack || journal.outcome.State != txn.StateRolledBack {
		t.Fatalf("journal %+v", journal)
	}
	if recovery.results[0].State != domain.StateRolledBack {
		t.Fatalf("results %+v", recovery.results)
	}
}

func TestUPD_016_snapshot_phase_is_aborted_and_reported_failed(t *testing.T) {
	journal, finisher, recovery := recoverWith(record(txn.StateSnapshot))
	if finisher.final != domain.StateFailed || len(journal.advanced) != 0 || journal.outcome.State != txn.StateAborted {
		t.Fatalf("finisher %+v journal %+v", finisher, journal)
	}
	cycle := pipeline.NewCycle("c", time.Time{}, true, []pipeline.Host{{Name: "local"}})
	if err := recovery.Run(context.Background(), cycle); err != nil {
		t.Fatal(err)
	}
	candidates := cycle.Hosts[0].Candidates
	if len(candidates) != 1 || candidates[0].Result.Container != "web" || candidates[0].Result.State != domain.StateFailed || candidates[0].Result.Reason != "interrupted" {
		t.Fatalf("candidates %+v", candidates)
	}
}

func TestUPD_016_handover_and_self_update_are_left_open(t *testing.T) {
	journal, finisher, _ := recoverWith(record(txn.StateHandover))
	if journal.finished || finisher.reason != "" {
		t.Fatal("handover transaction must not be recovered here")
	}
}
