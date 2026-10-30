package pipeline

import (
	"context"
	"testing"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type recordingHooks struct {
	calls []struct {
		name      txn.HookName
		container string
	}
}

func (r *recordingHooks) Run(_ context.Context, name txn.HookName, container domain.Container) txn.HookOutcome {
	r.calls = append(r.calls, struct {
		name      txn.HookName
		container string
	}{name, container.Name})
	return txn.HookOutcome{}
}

func TestPreCheckStageRunsForEveryCandidate(t *testing.T) {
	rec := &recordingHooks{}
	stage := NewPreCheckStage(map[string]txn.Hooks{"local": rec})
	cycle := &Cycle{Hosts: []*HostRun{{
		Host:      Host{Name: "local"},
		Reachable: true,
		Candidates: []*Candidate{
			{Container: domain.Container{Name: "a", ID: "1"}},
			{Container: domain.Container{Name: "b", ID: "2"}},
		},
	}}}
	if err := stage.Run(t.Context(), cycle); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 2 || rec.calls[0].name != txn.HookPreCheck || rec.calls[1].name != txn.HookPreCheck {
		t.Fatalf("calls: %+v", rec.calls)
	}
}

func TestPostCheckStageSkipsTransactionUpdatedAndRolledBack(t *testing.T) {
	rec := &recordingHooks{}
	stage := NewPostCheckStage(map[string]txn.Hooks{"local": rec})
	cycle := &Cycle{Hosts: []*HostRun{{
		Host:      Host{Name: "local"},
		Reachable: true,
		Candidates: []*Candidate{
			{Container: domain.Container{Name: "updated", ID: "1"}, Result: domain.Result{Transaction: "txn-1", State: domain.StateUpdated}},
			{Container: domain.Container{Name: "rolled", ID: "2"}, Result: domain.Result{Transaction: "txn-2", State: domain.StateRolledBack}},
			{Container: domain.Container{Name: "held", ID: "3"}, Result: domain.Result{Transaction: "txn-3", State: domain.StateHeld, Reason: "hook_deferred"}},
			{Container: domain.Container{Name: "plain", ID: "4"}, Result: domain.Result{State: domain.StateWouldUpdate}},
		},
	}}}
	if err := stage.Run(t.Context(), cycle); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, call := range rec.calls {
		names = append(names, call.container)
	}
	if len(names) != 2 || names[0] != "held" || names[1] != "plain" {
		t.Fatalf("post-check containers: %v", names)
	}
	for _, call := range rec.calls {
		if call.name != txn.HookPostCheck {
			t.Fatalf("want post-check, got %q", call.name)
		}
	}
}

func TestPostCheckStageUsesNoHooksForUnknownHost(t *testing.T) {
	stage := NewPostCheckStage(map[string]txn.Hooks{})
	cycle := &Cycle{Hosts: []*HostRun{{
		Host:       Host{Name: "remote"},
		Reachable:  true,
		Candidates: []*Candidate{{Container: domain.Container{Name: "a", ID: "1"}}},
	}}}
	if err := stage.Run(t.Context(), cycle); err != nil {
		t.Fatal(err)
	}
}
