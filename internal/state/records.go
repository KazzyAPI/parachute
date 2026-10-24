package state

import (
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

func (d Document) discardInvalid() Document {
	d.Transactions = slices.DeleteFunc(d.Transactions, func(record TransactionRecord) bool {
		return !txn.ValidID(record.ID) || !txn.State(record.State).Valid()
	})
	d.Snapshots = slices.DeleteFunc(d.Snapshots, func(record SnapshotRecord) bool {
		return !txn.ValidID(record.ID) || !txn.ValidID(record.Transaction)
	})
	d.Freezes = slices.DeleteFunc(d.Freezes, func(record Freeze) bool {
		return !txn.ValidID(record.Transaction) || !txn.ValidID(record.Snapshot)
	})
	d.Releases = slices.DeleteFunc(d.Releases, func(record Release) bool {
		return !txn.ValidID(record.Transaction)
	})
	return d
}
