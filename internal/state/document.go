package state

import (
	"slices"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
)

const Format = "parachute.state/v1"

type CycleInfo struct {
	ID      string    `json:"id"`
	Started time.Time `json:"started"`
	Ended   time.Time `json:"ended"`
}

type FirstSeen struct {
	Host      string    `json:"host"`
	Container string    `json:"container"`
	Digest    string    `json:"digest"`
	Time      time.Time `json:"time"`
}

type Approval struct {
	Host      string    `json:"host"`
	Container string    `json:"container"`
	Digest    string    `json:"digest"`
	Time      time.Time `json:"time"`
}

type Notified struct {
	Host      string    `json:"host"`
	Container string    `json:"container"`
	Digest    string    `json:"digest"`
	Reason    string    `json:"reason"`
	Time      time.Time `json:"time"`
}

type Document struct {
	Format              string              `json:"format"`
	LastCycle           *CycleInfo          `json:"last_cycle"`
	Results             []domain.Result     `json:"results"`
	FirstSeen           []FirstSeen         `json:"first_seen"`
	Approvals           []Approval          `json:"approvals"`
	Notified            []Notified          `json:"notified"`
	SupersededInstances []string            `json:"superseded_instances"`
	Transactions        []TransactionRecord `json:"transactions"`
	Snapshots           []SnapshotRecord    `json:"snapshots"`
	FailedDigests       []FailedDigest      `json:"failed_digests"`
	Freezes             []Freeze            `json:"freezes"`
	Releases            []Release           `json:"releases"`
}

func (d Document) clone() Document {
	d.Results = slices.Clone(d.Results)
	for index := range d.Results {
		d.Results[index].HoldUntil = clonePointer(d.Results[index].HoldUntil)
		d.Results[index].Drift = clonePointer(d.Results[index].Drift)
	}
	d.FirstSeen = slices.Clone(d.FirstSeen)
	d.Approvals = slices.Clone(d.Approvals)
	d.Notified = slices.Clone(d.Notified)
	d.SupersededInstances = slices.Clone(d.SupersededInstances)
	d.Transactions = slices.Clone(d.Transactions)
	for index := range d.Transactions {
		d.Transactions[index].Stacks = slices.Clone(d.Transactions[index].Stacks)
	}
	d.Snapshots = slices.Clone(d.Snapshots)
	d.FailedDigests = slices.Clone(d.FailedDigests)
	d.Freezes = slices.Clone(d.Freezes)
	for index := range d.Freezes {
		d.Freezes[index].Keys = slices.Clone(d.Freezes[index].Keys)
	}
	d.Releases = slices.Clone(d.Releases)
	for index := range d.Releases {
		d.Releases[index].Keys = slices.Clone(d.Releases[index].Keys)
	}
	return d.complete()
}

func (d Document) complete() Document {
	d.Format = Format
	d.Results = orEmpty(d.Results)
	d.FirstSeen = orEmpty(d.FirstSeen)
	d.Approvals = orEmpty(d.Approvals)
	d.Notified = orEmpty(d.Notified)
	d.SupersededInstances = orEmpty(d.SupersededInstances)
	d.Transactions = orEmpty(d.Transactions)
	d.Snapshots = orEmpty(d.Snapshots)
	d.FailedDigests = orEmpty(d.FailedDigests)
	d.Freezes = orEmpty(d.Freezes)
	d.Releases = orEmpty(d.Releases)
	return d
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func orEmpty[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}
