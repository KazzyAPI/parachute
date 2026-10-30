package state

import (
	"testing"

	"github.com/KazzyAPI/parachute/internal/domain"
)

func TestFreezeCoversOwnContainerByName(t *testing.T) {
	freeze := Freeze{Keys: []FreezeKey{{Host: "local", Container: "parachute-1"}}}
	own := domain.Container{Name: "parachute-1", Labels: map[string]string{"com.docker.compose.project": "ops"}}
	web := domain.Container{Name: "web-1", Labels: map[string]string{"com.docker.compose.project": "ops"}}
	if !freeze.Covers("local", own) || freeze.Covers("local", web) {
		t.Errorf("self freeze covers own=%v web=%v", freeze.Covers("local", own), freeze.Covers("local", web))
	}
}

func TestReleaseEndsSelfUpdateFreezeByMatchingKeys(t *testing.T) {
	keys := []FreezeKey{{Host: "local", Container: "parachute-1"}}
	freeze := Freeze{Transaction: "txn-20260101T000040Z-aaaa40", Keys: keys}
	release := Release{Transaction: "txn-20260101T000040Z-aaaa40", Keys: keys}
	if !ReleaseEndsFreeze(release, freeze) {
		t.Fatal("matching release should end freeze")
	}
	mismatch := Release{Transaction: freeze.Transaction, Keys: []FreezeKey{{Host: "local", Container: "other"}}}
	if ReleaseEndsFreeze(mismatch, freeze) {
		t.Fatal("container name mismatch should not end freeze")
	}
}
