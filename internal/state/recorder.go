package state

import (
	"context"

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
		return nil
	})
}
