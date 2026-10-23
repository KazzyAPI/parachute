package notify

import (
	"slices"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/state"
)

const reasonApprovalRequired = "approval_required"

type LedgerStore interface {
	Update(change func(*state.Document) error) error
}

type Ledger struct {
	store    LedgerStore
	reminder time.Duration
}

func NewLedger(store LedgerStore, reminderDays float64) *Ledger {
	return &Ledger{store: store, reminder: time.Duration(reminderDays * float64(24*time.Hour))}
}

func (l *Ledger) Hidden(report domain.Report) (func(domain.Result) bool, error) {
	hidden := map[state.Notified]bool{}
	err := l.store.Update(func(document *state.Document) error {
		notified := slices.DeleteFunc(document.Notified, func(entry state.Notified) bool { return !l.keep(entry, report) })
		for _, result := range report.Results() {
			if result.State != domain.StateHeld {
				continue
			}
			key := keyOf(result)
			index := slices.IndexFunc(notified, func(entry state.Notified) bool { return withoutTime(entry) == key })
			switch {
			case index < 0:
				key.Time = report.Started
				notified = append(notified, key)
			case result.Reason == reasonApprovalRequired && report.Started.Sub(notified[index].Time) >= l.reminder:
				notified[index].Time = report.Started
			default:
				hidden[key] = true
			}
		}
		document.Notified = notified
		return nil
	})
	return func(result domain.Result) bool { return hidden[keyOf(result)] }, err
}

func (l *Ledger) keep(entry state.Notified, report domain.Report) bool {
	for _, host := range report.Hosts {
		if host.Name != entry.Host {
			continue
		}
		if !host.Reachable {
			return true
		}
		return slices.ContainsFunc(host.Results, func(result domain.Result) bool {
			return result.Container == entry.Container && (result.CandidateDigest == "" || keyOf(result) == withoutTime(entry))
		})
	}
	return true
}

func keyOf(result domain.Result) state.Notified {
	return state.Notified{Host: result.Host, Container: result.Container, Digest: result.CandidateDigest, Reason: result.Reason}
}

func withoutTime(entry state.Notified) state.Notified {
	entry.Time = time.Time{}
	return entry
}
