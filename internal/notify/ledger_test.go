package notify

import (
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/state"
)

type memoryStore struct{ document state.Document }

func (m *memoryStore) Update(change func(*state.Document) error) error { return change(&m.document) }

func heldReport(started time.Time, reason string) domain.Report {
	result := domain.Result{Host: "local", Container: "db", CandidateDigest: "sha256:1", State: domain.StateHeld, Reason: reason}
	return domain.Report{Started: started, Hosts: []domain.HostReport{{Name: "local", Reachable: true, Results: []domain.Result{result}}}}
}

func TestLedgerRemindsApprovalsAndShowsOtherHoldsOnce(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for reason, want := range map[string][]bool{
		reasonApprovalRequired: {true, false, true, false, true},
		"delay":                {true, false, false, false, false},
	} {
		ledger := NewLedger(&memoryStore{}, 0.001)
		for cycle, shown := range want {
			report := heldReport(start.Add(time.Duration(cycle)*time.Minute), reason)
			hidden, err := ledger.Hidden(report)
			if err != nil {
				t.Fatal(err)
			}
			if got := !hidden(report.Results()[0]); got != shown {
				t.Errorf("%s cycle %d: shown %t, want %t", reason, cycle, got, shown)
			}
		}
	}
}
