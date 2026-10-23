package policy

import (
	"slices"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/state"
)

type DelayRule struct {
	store Store
	days  float64
}

func NewDelayRule(store Store, days float64) *DelayRule {
	return &DelayRule{store: store, days: days}
}

func (r *DelayRule) Hold(s Subject) (domain.Hold, bool, error) {
	seen, err := r.firstSeen(s)
	if err != nil {
		return domain.Hold{}, false, err
	}
	days := s.Overrides.DelayDays(r.days)
	until := seen.Add(time.Duration(days * float64(24*time.Hour)))
	return domain.Hold{Reason: "delay", Until: &until}, days > 0 && s.Now.Before(until), nil
}

func (r *DelayRule) firstSeen(s Subject) (time.Time, error) {
	digest := s.Result.CandidateDigest
	for _, record := range r.store.Current().FirstSeen {
		if record.Host == s.Host && record.Container == s.Container.Name && record.Digest == digest {
			if record.Time.After(s.Now) {
				return s.Now, nil
			}
			return record.Time, nil
		}
	}
	seen := s.Now.UTC()
	err := r.store.Update(func(document *state.Document) error {
		document.FirstSeen = append(slices.DeleteFunc(document.FirstSeen, func(record state.FirstSeen) bool {
			return record.Host == s.Host && record.Container == s.Container.Name
		}), state.FirstSeen{Host: s.Host, Container: s.Container.Name, Digest: digest, Time: seen})
		return nil
	})
	return seen, err
}
