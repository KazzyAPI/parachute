package policy

import "github.com/KazzyAPI/parachute/internal/domain"

const reasonPreviousAttemptFailed = "previous_attempt_failed"

type FailureRule struct {
	store Store
}

func NewFailureRule(store Store) *FailureRule {
	return &FailureRule{store: store}
}

func (r *FailureRule) Hold(s Subject) (domain.Hold, bool, error) {
	document := r.store.Current()
	for _, freeze := range document.ActiveFreezes() {
		if freeze.Covers(s.Host, s.Container) {
			return domain.Hold{Reason: reasonPreviousAttemptFailed, Detail: "frozen by " + freeze.Transaction}, true, nil
		}
	}
	hold := domain.Hold{Reason: reasonPreviousAttemptFailed, Detail: "digest failed in an earlier transaction"}
	return hold, !s.Approved && document.Failed(s.Host, s.Container.Name, s.Result.CandidateDigest), nil
}
