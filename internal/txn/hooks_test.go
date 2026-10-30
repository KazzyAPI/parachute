package txn

import "testing"

func TestHookOutcomeFailed(t *testing.T) {
	for _, tc := range []struct {
		outcome HookOutcome
		want    bool
	}{
		{HookOutcome{}, false},
		{HookOutcome{Ran: true, ExitCode: 0}, false},
		{HookOutcome{Ran: true, ExitCode: 75, Deferred: true}, false},
		{HookOutcome{Ran: true, ExitCode: 1}, true},
		{HookOutcome{TimedOut: true}, true},
		{HookOutcome{Ran: true, TimedOut: true, ExitCode: 0}, true},
	} {
		if got := tc.outcome.Failed(); got != tc.want {
			t.Fatalf("%+v Failed() = %t, want %t", tc.outcome, got, tc.want)
		}
	}
}
