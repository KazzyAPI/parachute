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
	results := report.Results()
	if results == nil {
		results = []domain.Result{}
	}
	return r.store.Save(Document{
		Format:    Format,
		LastCycle: &CycleInfo{ID: report.CycleID, Started: report.Started, Ended: report.Ended},
		Results:   results,
	})
}
