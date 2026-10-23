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
	Format              string          `json:"format"`
	LastCycle           *CycleInfo      `json:"last_cycle"`
	Results             []domain.Result `json:"results"`
	FirstSeen           []FirstSeen     `json:"first_seen"`
	Approvals           []Approval      `json:"approvals"`
	Notified            []Notified      `json:"notified"`
	SupersededInstances []string        `json:"superseded_instances"`
}

func (d Document) clone() Document {
	d.Results = slices.Clone(d.Results)
	d.FirstSeen = slices.Clone(d.FirstSeen)
	d.Approvals = slices.Clone(d.Approvals)
	d.Notified = slices.Clone(d.Notified)
	d.SupersededInstances = slices.Clone(d.SupersededInstances)
	return d.complete()
}

func (d Document) complete() Document {
	d.Results = orEmpty(d.Results)
	d.FirstSeen = orEmpty(d.FirstSeen)
	d.Approvals = orEmpty(d.Approvals)
	d.Notified = orEmpty(d.Notified)
	d.SupersededInstances = orEmpty(d.SupersededInstances)
	return d
}

func orEmpty[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}
