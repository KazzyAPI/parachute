package state

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/internal/txn"
)

func pruneID(prefix string, index int) string {
	return fmt.Sprintf("%s-20200101T000000Z-%06x", prefix, index)
}

func TestFinishPrunesOldTransactionsAndKeepsOpenFreezeAndLatest(t *testing.T) {
	store := NewStore("/s", memoryFiles{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	journal := NewJournal(store, func() time.Time { return time.Time{} })
	seed := func(document *Document) error {
		document.Transactions = append(document.Transactions,
			TransactionRecord{ID: pruneID("txn", 0), Host: "h", Stacks: []string{"rare"}, State: "committed"},
			TransactionRecord{ID: pruneID("txn", 1), Host: "h", Stacks: []string{"s"}, State: "rollback_failed"},
			TransactionRecord{ID: pruneID("txn", 2), Host: "h", Stacks: []string{"s"}, State: "update"},
		)
		document.Freezes = append(document.Freezes, Freeze{Transaction: pruneID("txn", 1), Snapshot: pruneID("snap", 1), Keys: []FreezeKey{{Host: "h", Stack: "s"}}})
		document.Snapshots = append(document.Snapshots, SnapshotRecord{ID: pruneID("snap", 1), Host: "h", Stack: "s", Transaction: pruneID("txn", 1)})
		for index := 10; index < 10+MaxFinishedTransactions+50; index++ {
			document.Transactions = append(document.Transactions, TransactionRecord{ID: pruneID("txn", index), Host: "h", Stacks: []string{"x"}, State: "committed"})
		}
		for index := 10; index < 10+MaxSnapshotsPerStack+5; index++ {
			document.Snapshots = append(document.Snapshots, SnapshotRecord{ID: pruneID("snap", index), Host: "h", Stack: "x", Transaction: pruneID("txn", index)})
		}
		return nil
	}
	if err := store.Update(seed); err != nil {
		t.Fatal(err)
	}
	if err := journal.Finish(context.Background(), pruneID("txn", 2), txn.Outcome{State: txn.StateCommitted}); err != nil {
		t.Fatal(err)
	}
	document := store.Current()
	ids := map[string]bool{}
	for _, record := range document.Transactions {
		ids[record.ID] = true
	}
	if !ids[pruneID("txn", 0)] || !ids[pruneID("txn", 1)] || !ids[pruneID("txn", 2)] {
		t.Fatalf("latest, frozen, or just finished transaction was pruned: %v", ids)
	}
	if want := MaxFinishedTransactions + 3; len(document.Transactions) != want {
		t.Fatalf("%d transactions, want %d", len(document.Transactions), want)
	}
	if len(document.Freezes) != 1 {
		t.Fatal("freeze was pruned")
	}
	perStack := map[string]int{}
	frozen := false
	for _, record := range document.Snapshots {
		perStack[record.Stack]++
		frozen = frozen || record.ID == pruneID("snap", 1)
	}
	if perStack["x"] != MaxSnapshotsPerStack || !frozen {
		t.Fatalf("snapshot records %v frozen kept %v", perStack, frozen)
	}
}

func TestFinishKeepsOpenTransactionsBeyondTheBound(t *testing.T) {
	store := NewStore("/s", memoryFiles{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	journal := NewJournal(store, func() time.Time { return time.Time{} })
	if err := store.Update(func(document *Document) error {
		document.Transactions = append(document.Transactions, TransactionRecord{ID: pruneID("txn", 0), Host: "h", Stacks: []string{"o"}, State: "verify"})
		for index := 1; index < MaxFinishedTransactions+20; index++ {
			document.Transactions = append(document.Transactions, TransactionRecord{ID: pruneID("txn", index), Host: "h", Stacks: []string{"x"}, State: "committed"})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := journal.Finish(context.Background(), pruneID("txn", 1), txn.Outcome{State: txn.StateCommitted}); err != nil {
		t.Fatal(err)
	}
	if open := journal.OpenTransactionIDs(); len(open) != 1 {
		t.Fatalf("open transactions %v", open)
	}
}
