package state

import (
	"slices"

	"github.com/KazzyAPI/parachute/internal/txn"
)

const (
	MaxFinishedTransactions = 100
	MaxSnapshotsPerStack    = 100
)

func prune(document *Document) {
	keep := map[string]bool{}
	for _, freeze := range document.Freezes {
		keep[freeze.Transaction] = true
	}
	for _, release := range document.Releases {
		keep[release.Transaction] = true
	}
	for _, link := range document.ServiceTransactions {
		keep[link.Transaction] = true
	}
	frozenSnapshots := map[string]bool{}
	for _, freeze := range document.ActiveFreezes() {
		frozenSnapshots[freeze.Snapshot] = true
	}
	perStack := map[FreezeKey]int{}
	for index := len(document.Snapshots) - 1; index >= 0; index-- {
		record := document.Snapshots[index]
		key := FreezeKey{Host: record.Host, Stack: record.Stack}
		perStack[key]++
		if perStack[key] > MaxSnapshotsPerStack && !frozenSnapshots[record.ID] {
			document.Snapshots = slices.Delete(document.Snapshots, index, index+1)
		}
	}
	finished := 0
	stacks := map[FreezeKey]bool{}
	for index := len(document.Transactions) - 1; index >= 0; index-- {
		record := document.Transactions[index]
		if txn.State(record.State).Open() {
			keep[record.ID] = true
			continue
		}
		finished++
		if finished <= MaxFinishedTransactions {
			keep[record.ID] = true
		}
		for _, stack := range record.Stacks {
			key := FreezeKey{Host: record.Host, Stack: stack}
			if !stacks[key] {
				stacks[key] = true
				keep[record.ID] = true
			}
		}
	}
	document.Transactions = slices.DeleteFunc(document.Transactions, func(record TransactionRecord) bool { return !keep[record.ID] })
}
