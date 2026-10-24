package state

import (
	"context"
	"time"

	"github.com/KazzyAPI/parachute/internal/txn"
)

type Journal struct {
	store *Store
	clock func() time.Time
}

func NewJournal(store *Store, clock func() time.Time) *Journal {
	return &Journal{store: store, clock: clock}
}

func (j *Journal) Begin(ctx context.Context, record txn.OpenRecord) error {
	return j.store.Update(func(document *Document) error {
		document.Transactions = append(document.Transactions, TransactionRecord{
			ID: record.ID, Host: record.Host, Stacks: record.Stacks, State: string(record.State),
			Snapshot: record.Snapshot, SelfUpdate: record.SelfUpdate, Time: j.clock(),
		})
		return nil
	})
}

func (j *Journal) Advance(ctx context.Context, id string, state txn.State) error {
	return j.store.Update(func(document *Document) error {
		for index, record := range document.Transactions {
			if record.ID != id {
				continue
			}
			document.Transactions[index].State = string(state)
			return nil
		}
		return nil
	})
}

func (j *Journal) Finish(ctx context.Context, id string, outcome txn.Outcome) error {
	return j.store.Update(func(document *Document) error {
		for index, record := range document.Transactions {
			if record.ID != id {
				continue
			}
			document.Transactions[index].State = string(outcome.State)
			document.Transactions[index].Result = string(outcome.Result)
			document.Transactions[index].Reason = string(outcome.Reason)
			return nil
		}
		return nil
	})
}

func (j *Journal) Open(ctx context.Context) ([]txn.OpenRecord, error) {
	document := j.store.Current()
	var open []txn.OpenRecord
	for _, record := range document.Transactions {
		state := txn.State(record.State)
		if !state.Open() {
			continue
		}
		open = append(open, txn.OpenRecord{
			ID: record.ID, Host: record.Host, Stacks: record.Stacks, State: state,
			Snapshot: record.Snapshot, SelfUpdate: record.SelfUpdate,
		})
	}
	return open, nil
}

func (j *Journal) RecordSnapshot(host, stack, transaction, snapshot string) error {
	return j.store.Update(func(document *Document) error {
		document.Snapshots = append(document.Snapshots, SnapshotRecord{
			ID: snapshot, Host: host, Stack: stack, Transaction: transaction, Time: j.clock(),
		})
		return nil
	})
}
