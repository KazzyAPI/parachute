package policy

import "github.com/KazzyAPI/parachute/internal/domain"

// A digest that already failed is held as previous_attempt_failed (POL-010), not as needing approval again.
type SelfRule struct{ store Store }

func NewSelfRule(store Store) *SelfRule { return &SelfRule{store: store} }

func (r *SelfRule) Hold(s Subject) (domain.Hold, bool, error) {
	hold := domain.Hold{Reason: reasonApprovalRequired, Detail: "self-update needs approval"}
	held := s.Container.Own && !s.Approved && !r.store.Current().Failed(s.Host, s.Container.Name, s.Result.CandidateDigest)
	return hold, held, nil
}
