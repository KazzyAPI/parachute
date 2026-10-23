package policy

import "github.com/KazzyAPI/parachute/internal/domain"

type SelfRule struct{}

func NewSelfRule() *SelfRule { return &SelfRule{} }

func (r *SelfRule) Hold(s Subject) (domain.Hold, bool, error) {
	return domain.Hold{Reason: reasonApprovalRequired, Detail: "self-update needs approval"}, s.Container.Own && !s.Approved, nil
}
