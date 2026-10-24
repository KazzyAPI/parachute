package policy

import (
	"testing"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/state"
)

type fakeStore struct {
	document state.Document
}

func (f *fakeStore) Current() state.Document { return f.document }

func (f *fakeStore) Update(change func(*state.Document) error) error { return change(&f.document) }

func TestFailureRule(t *testing.T) {
	keys := []state.FreezeKey{{Host: "local", Stack: "shop"}}
	frozen := state.Document{Freezes: []state.Freeze{{Transaction: "txn-1", Keys: keys}}}
	released := frozen
	released.Releases = []state.Release{{Transaction: "txn-1", Keys: keys}}
	mismatched := frozen
	mismatched.Releases = []state.Release{{Transaction: "txn-1", Keys: []state.FreezeKey{{Host: "local", Stack: "other"}}}}
	failed := state.Document{FailedDigests: []state.FailedDigest{{Host: "local", Container: "web", Digest: "sha256:a"}}}
	compose := domain.Container{Name: "web", Labels: map[string]string{"com.docker.compose.project": "shop"}}
	plain := domain.Container{Name: "web"}
	cases := []struct {
		name      string
		document  state.Document
		container domain.Container
		digest    string
		approved  bool
		held      bool
	}{
		{"frozen stack", frozen, compose, "sha256:z", true, true},
		{"released freeze", released, compose, "sha256:z", false, false},
		{"release with other keys", mismatched, compose, "sha256:z", false, true},
		{"other stack", frozen, plain, "sha256:z", false, false},
		{"failed digest", failed, plain, "sha256:a", false, true},
		{"approved failed digest", failed, plain, "sha256:a", true, false},
		{"new digest", failed, plain, "sha256:b", false, false},
	}
	for _, c := range cases {
		hold, held, err := NewFailureRule(&fakeStore{document: c.document}).Hold(Subject{
			Host: "local", Container: c.container, Result: domain.Result{CandidateDigest: c.digest}, Approved: c.approved,
		})
		if err != nil || held != c.held || held && hold.Reason != "previous_attempt_failed" {
			t.Errorf("%s: held %v reason %q err %v, want held %v", c.name, held, hold.Reason, err, c.held)
		}
	}
}
