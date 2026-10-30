package status

import (
	"slices"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/state"
)

type identity struct{}

func (identity) Redact(text string) string { return text }

func TestFreezeListsKeyedStacksAndCoveredContainers(t *testing.T) {
	web := domain.Container{Name: "shop-web-1", Labels: map[string]string{"com.docker.compose.project": "shop"}}
	run := &pipeline.HostRun{Host: pipeline.Host{Name: "local"}, Reachable: true, Containers: []domain.Container{web}}
	freeze := state.Freeze{Transaction: "txn-1", Snapshot: "snap-1", Keys: []state.FreezeKey{{Host: "local", Stack: "shop"}, {Host: "local", Stack: "gone"}, {Host: "remote", Stack: "far"}}}
	stacks := NewBuilder(nil, nil, identity{}, false).freeze([]Stack{{Name: "shop", Frozen: []Freeze{}, Services: []Service{{}}}}, run, []state.Freeze{freeze})
	if len(stacks) != 2 || stacks[0].Name != "shop" || stacks[1].Name != "gone" || len(stacks[1].Services) != 0 {
		t.Fatalf("stacks %+v", stacks)
	}
	for _, stack := range stacks {
		if len(stack.Frozen) != 1 || stack.Frozen[0].Transaction != "txn-1" || !slices.Equal(stack.Frozen[0].Containers, []string{"shop-web-1"}) {
			t.Errorf("stack %s frozen %+v", stack.Name, stack.Frozen)
		}
	}
}

func TestTransactionHidesResultWhileOpen(t *testing.T) {
	when := time.Unix(10, 0)
	open := transaction(state.TransactionRecord{ID: "txn-1", State: "verify", Result: "x", Time: when})
	done := transaction(state.TransactionRecord{ID: "txn-2", State: "rolled_back", Result: "rolled_back", Reason: "verify_failed", Time: when})
	if open.Result != nil || open.Reason != nil || *done.Result != "rolled_back" || *done.Reason != "verify_failed" {
		t.Errorf("open %+v done %+v", open, done)
	}
}
