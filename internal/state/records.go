package state

import (
	"log/slog"
	"slices"
	"time"

	"github.com/KazzyAPI/parachute/internal/txn"
)

type TransactionRecord struct {
	ID         string    `json:"id"`
	Host       string    `json:"host"`
	Stacks     []string  `json:"stacks"`
	State      string    `json:"state"`
	Snapshot   string    `json:"snapshot,omitempty"`
	SelfUpdate bool      `json:"self_update,omitempty"`
	Result     string    `json:"result,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	Time       time.Time `json:"time"`
}

type SnapshotRecord struct {
	ID          string    `json:"id"`
	Host        string    `json:"host"`
	Stack       string    `json:"stack"`
	Transaction string    `json:"transaction"`
	Time        time.Time `json:"time"`
}

type FailedDigest struct {
	Host      string `json:"host"`
	Container string `json:"container"`
	Digest    string `json:"digest"`
}

type FreezeKey struct {
	Host      string `json:"host"`
	Stack     string `json:"stack,omitempty"`
	Container string `json:"container,omitempty"`
}

type Freeze struct {
	Transaction string      `json:"transaction"`
	Snapshot    string      `json:"snapshot"`
	Keys        []FreezeKey `json:"keys"`
}

type Release struct {
	Transaction string      `json:"transaction"`
	Keys        []FreezeKey `json:"keys"`
	Time        time.Time   `json:"time"`
}

func (d Document) discardInvalid(logger *slog.Logger) Document {
	d.Transactions = discard(logger, "transaction", d.Transactions, func(record TransactionRecord) (string, bool) {
		return record.ID, txn.ValidID(record.ID) && txn.State(record.State).Valid()
	})
	d.Snapshots = discard(logger, "snapshot", d.Snapshots, func(record SnapshotRecord) (string, bool) {
		return record.ID, txn.ValidID(record.ID) && txn.ValidID(record.Transaction)
	})
	d.Freezes = discard(logger, "freeze", d.Freezes, func(record Freeze) (string, bool) {
		return record.Transaction, txn.ValidID(record.Transaction) && txn.ValidID(record.Snapshot) && validKeys(record.Keys)
	})
	d.Releases = discard(logger, "release", d.Releases, func(record Release) (string, bool) {
		return record.Transaction, txn.ValidID(record.Transaction) && validKeys(record.Keys)
	})
	return d
}

func discard[T any](logger *slog.Logger, kind string, records []T, check func(T) (string, bool)) []T {
	return slices.DeleteFunc(records, func(record T) bool {
		id, valid := check(record)
		if !valid {
			logger.Info("discarded invalid state record", "kind", kind, "id", id)
		}
		return !valid
	})
}

func validKeys(keys []FreezeKey) bool {
	return len(keys) > 0 && !slices.ContainsFunc(keys, func(key FreezeKey) bool { return key.Host == "" })
}
