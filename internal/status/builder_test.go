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

func TestFreezeSelfUpdateListsOwnContainerOnly(t *testing.T) {
	own := domain.Container{Name: "parachute-1", Labels: map[string]string{"com.docker.compose.project": "ops"}}
	web := domain.Container{Name: "web-1", Labels: map[string]string{"com.docker.compose.project": "ops"}}
	run := &pipeline.HostRun{Host: pipeline.Host{Name: "local"}, Reachable: true, Containers: []domain.Container{own, web}}
	freeze := state.Freeze{Transaction: "txn-20260101T000020Z-aaaa20", Keys: []state.FreezeKey{{Host: "local", Container: "parachute-1"}}}
	stacks := NewBuilder(nil, nil, identity{}, false).freeze([]Stack{{Name: "ops"}}, run, []state.Freeze{freeze})
	if len(stacks) != 1 || len(stacks[0].Frozen) != 1 {
		t.Fatalf("stacks %+v", stacks)
	}
	if !slices.Equal(stacks[0].Frozen[0].Containers, []string{"parachute-1"}) {
		t.Errorf("frozen containers %+v", stacks[0].Frozen[0].Containers)
	}
}

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

func TestBackupsFillsNewestSnapshotPerStack(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	snapshots := []state.SnapshotRecord{
		{ID: "snap-old", Host: "local", Stack: "web", Time: base},
		{ID: "snap-new", Host: "local", Stack: "web", Time: base.Add(time.Hour)},
		{ID: "snap-other", Host: "remote", Stack: "web", Time: base.Add(2 * time.Hour)},
	}
	stacks := []Stack{{Name: "web"}, {Name: "db"}}
	NewBuilder(nil, nil, identity{}, false).backups(stacks, "local", snapshots)
	if stacks[0].LastBackup == nil || stacks[0].LastBackup.Snapshot != "snap-new" || !stacks[0].LastBackup.Valid {
		t.Fatalf("web %+v", stacks[0].LastBackup)
	}
	if stacks[1].LastBackup != nil {
		t.Fatalf("db %+v", stacks[1].LastBackup)
	}
}
