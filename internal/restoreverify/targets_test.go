package restoreverify

import (
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/internal/state"
)

func TestSelectTargetsNewestPerStack(t *testing.T) {
	document := state.Document{Snapshots: []state.SnapshotRecord{
		{ID: "snap-20260201T120000Z-aaaaaa", Host: "local", Stack: "shop", Transaction: "txn-20260201T120000Z-aaaaaa", Time: time.Unix(1, 0)},
		{ID: "snap-20260201T130000Z-bbbbbb", Host: "local", Stack: "shop", Transaction: "txn-20260201T130000Z-bbbbbb", Time: time.Unix(2, 0)},
	}}
	targets := SelectTargets(document, Options{Retention: 3})
	if len(targets) != 1 || targets[0].Snapshot.ID != "snap-20260201T130000Z-bbbbbb" {
		t.Fatalf("targets %+v", targets)
	}
}

func TestSelectTargetsStackFilter(t *testing.T) {
	document := state.Document{Snapshots: []state.SnapshotRecord{
		{ID: "snap-20260201T120000Z-aaaaaa", Host: "local", Stack: "shop", Transaction: "txn-20260201T120000Z-aaaaaa"},
		{ID: "snap-20260201T120000Z-bbbbbb", Host: "local", Stack: "other", Transaction: "txn-20260201T120000Z-bbbbbb"},
	}}
	targets := SelectTargets(document, Options{Stack: "shop", Retention: 3})
	if len(targets) != 1 || targets[0].Stack != "shop" {
		t.Fatalf("targets %+v", targets)
	}
}
