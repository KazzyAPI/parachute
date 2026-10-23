package policy

import (
	"log/slog"
	"slices"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/state"
)

type Records struct {
	store  Store
	logger *slog.Logger
}

func NewRecords(store Store, logger *slog.Logger) *Records {
	return &Records{store: store, logger: logger}
}

func (r *Records) Prune(host string, containers []domain.Container) error {
	ids, names := map[string]bool{}, map[string]bool{}
	for _, c := range containers {
		ids[c.ID], names[c.Name] = true, true
	}
	staleApproval := func(record state.Approval) bool { return record.Host == host && !ids[record.Container] }
	staleSeen := func(record state.FirstSeen) bool { return record.Host == host && !names[record.Container] }
	current := r.store.Current()
	if !slices.ContainsFunc(current.Approvals, staleApproval) && !slices.ContainsFunc(current.FirstSeen, staleSeen) {
		return nil
	}
	return r.store.Update(func(document *state.Document) error {
		document.Approvals = slices.DeleteFunc(document.Approvals, func(record state.Approval) bool {
			if staleApproval(record) {
				r.logger.Info("approval discarded, its container no longer exists", "host", host, "container_id", record.Container, "digest", record.Digest)
				return true
			}
			return false
		})
		document.FirstSeen = slices.DeleteFunc(document.FirstSeen, staleSeen)
		return nil
	})
}
