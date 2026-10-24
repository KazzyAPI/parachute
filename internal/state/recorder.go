package state

import (
	"context"
	"slices"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type Recorder struct {
	store *Store
}

func NewRecorder(store *Store) *Recorder {
	return &Recorder{store: store}
}

func (r *Recorder) Publish(_ context.Context, report domain.Report) error {
	return r.store.Update(func(document *Document) error {
		document.LastCycle = &CycleInfo{ID: report.CycleID, Started: report.Started, Ended: report.Ended}
		document.Results = orEmpty(report.Results())
		for _, result := range document.Results {
			recordResult(document, result)
		}
		return nil
	})
}

func recordResult(document *Document, result domain.Result) {
	if result.Transaction != "" {
		document.ServiceTransactions = slices.DeleteFunc(document.ServiceTransactions, func(record ServiceTransaction) bool {
			return record.Host == result.Host && record.Container == result.Container
		})
		document.ServiceTransactions = append(document.ServiceTransactions, ServiceTransaction{Host: result.Host, Container: result.Container, Transaction: result.Transaction})
	}
	if result.CandidateDigest == "" || result.State != domain.StateRolledBack && result.State != domain.StateRollbackFailed {
		return
	}
	if !document.Failed(result.Host, result.Container, result.CandidateDigest) {
		document.FailedDigests = append(document.FailedDigests, FailedDigest{Host: result.Host, Container: result.Container, Digest: result.CandidateDigest})
	}
	document.Approvals = slices.DeleteFunc(document.Approvals, func(approval Approval) bool {
		return approval.Host == result.Host && approval.Container == result.ContainerID && approval.Digest == result.CandidateDigest
	})
}
