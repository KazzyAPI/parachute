package state

import (
	"context"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/txn"
)

func TestFinishSelfUpdateRollbackFailedFreezesOwnContainerOnly(t *testing.T) {
	store := NewStore("/s", memoryFiles{}, nil)
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	journal := NewJournal(store, func() time.Time { return time.Unix(0, 0) })
	ctx := context.Background()
	if err := journal.Begin(ctx, txn.OpenRecord{
		ID: "txn-20260101T000010Z-aaaa10", Host: "local", Stacks: []string{"ops"}, SelfUpdate: true, OwnContainer: "parachute-1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := journal.Finish(ctx, "txn-20260101T000010Z-aaaa10", txn.Outcome{State: txn.StateRollbackFail, Result: domain.StateRollbackFailed, Reason: "interrupted"}); err != nil {
		t.Fatal(err)
	}
	document := store.Current()
	if len(document.Freezes) != 1 {
		t.Fatalf("freezes %+v", document.Freezes)
	}
	freeze := document.Freezes[0]
	if freeze.Transaction != "txn-20260101T000010Z-aaaa10" || len(freeze.Keys) != 1 || freeze.Keys[0].Host != "local" || freeze.Keys[0].Container != "parachute-1" || freeze.Keys[0].Stack != "" {
		t.Errorf("freeze %+v", freeze)
	}
}

func TestFinishOrdinaryRollbackFailedStillFreezesStacks(t *testing.T) {
	store := NewStore("/s", memoryFiles{}, nil)
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	journal := NewJournal(store, func() time.Time { return time.Unix(0, 0) })
	ctx := context.Background()
	if err := journal.Begin(ctx, txn.OpenRecord{ID: "txn-20260101T000011Z-aaaa11", Host: "local", Stacks: []string{"shop", "cache"}}); err != nil {
		t.Fatal(err)
	}
	if err := journal.Finish(ctx, "txn-20260101T000011Z-aaaa11", txn.Outcome{State: txn.StateRollbackFail, Result: domain.StateRollbackFailed, Reason: "restore_failed"}); err != nil {
		t.Fatal(err)
	}
	freeze := store.Current().Freezes[0]
	if len(freeze.Keys) != 2 || freeze.Keys[0].Stack != "shop" || freeze.Keys[1].Stack != "cache" {
		t.Errorf("freeze keys %+v", freeze.Keys)
	}
}

func TestFinishRecordsSupersededInstance(t *testing.T) {
	store := NewStore("/s", memoryFiles{}, nil)
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	journal := NewJournal(store, func() time.Time { return time.Unix(0, 0) })
	ctx := context.Background()
	if err := journal.Begin(ctx, txn.OpenRecord{ID: "txn-20260101T000012Z-aaaa12", Host: "local", Stacks: []string{"ops"}, SelfUpdate: true, OwnContainer: "parachute-1"}); err != nil {
		t.Fatal(err)
	}
	if err := journal.Finish(ctx, "txn-20260101T000012Z-aaaa12", txn.Outcome{
		State: txn.StateRollbackFail, Result: domain.StateRollbackFailed, Reason: "interrupted", Superseded: "deadbeef",
	}); err != nil {
		t.Fatal(err)
	}
	if got := store.Current().SupersededInstances; len(got) != 1 || got[0] != "deadbeef" {
		t.Errorf("superseded_instances %v", got)
	}
}
