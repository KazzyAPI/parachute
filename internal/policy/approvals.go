package policy

import (
	"log/slog"
	"slices"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/state"
)

type Store interface {
	Current() state.Document
	Update(change func(*state.Document) error) error
}

type Approvals struct {
	store  Store
	logger *slog.Logger
}

func NewApprovals(store Store, logger *slog.Logger) *Approvals {
	return &Approvals{store: store, logger: logger}
}

func (a *Approvals) Approved(host string, c domain.Container, digest string) (bool, error) {
	approved, lapsed := false, false
	for _, record := range a.store.Current().Approvals {
		if record.Host != host || record.Container != c.ID {
			continue
		}
		if record.Digest == digest {
			approved = true
			continue
		}
		lapsed = true
		a.logger.Info("approval discarded, the candidate digest changed", "host", host, "container", c.Name, "digest", record.Digest)
	}
	if !lapsed {
		return approved, nil
	}
	return approved, a.store.Update(func(document *state.Document) error {
		document.Approvals = slices.DeleteFunc(document.Approvals, func(record state.Approval) bool {
			return record.Host == host && record.Container == c.ID && record.Digest != digest
		})
		return nil
	})
}
