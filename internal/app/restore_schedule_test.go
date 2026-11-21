package app

import (
	"context"
	"strings"
	"testing"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/restoreverify"
)

type recordingAnnouncer struct {
	level domain.Level
	body  string
}

func (r *recordingAnnouncer) Announce(_ context.Context, level domain.Level, body string) {
	r.level, r.body = level, body
}

func (*recordingAnnouncer) Wait() {}

func TestRST_006_announcement_names_stack_and_result(t *testing.T) {
	announcer := &recordingAnnouncer{}
	announceRestoreVerify(context.Background(), announcer, restoreverify.Document{
		ID: "rv-20260101T000000Z-abcdef",
		Stacks: []restoreverify.StackRecord{
			{Host: "local", Stack: "shop", Result: "pass"},
		},
	})
	if announcer.level != domain.LevelInfo || !strings.Contains(announcer.body, "shop: pass") {
		t.Fatalf("%v %q", announcer.level, announcer.body)
	}
	announceRestoreVerify(context.Background(), announcer, restoreverify.Document{
		ID: "rv-20260101T000000Z-abcdef",
		Stacks: []restoreverify.StackRecord{
			{Host: "local", Stack: "shop", Result: "fail", Reason: "snapshot_invalid"},
		},
	})
	if announcer.level != domain.LevelWarn || !strings.Contains(announcer.body, "fail (snapshot_invalid)") {
		t.Fatalf("%v %q", announcer.level, announcer.body)
	}
}
