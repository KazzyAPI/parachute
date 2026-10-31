package backup

import (
	"context"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/mount"
)

func TestSkippedByContainer(t *testing.T) {
	manifest := Manifest{Skipped: []SkippedRecord{{Container: "web", Destination: "/data"}}}
	skipped := skippedByContainer(manifest)
	if !skipped["web"]["/data"] {
		t.Fatalf("skipped %+v", skipped)
	}
}

func TestGuardRestoreTargetRejectsSystemBind(t *testing.T) {
	s := &Snapshotter{
		hostFS:   fakeHostFS{"/host-etc": {Path: "/etc", Backable: true}},
		settings: Settings{Volume: "/srv/parachute"},
	}
	err := s.guardRestoreTarget(context.Background(), mount.Mount{Type: mount.TypeBind, Source: "/host-etc"})
	if err == nil || !strings.Contains(err.Error(), reasonSnapshotInvalid) {
		t.Fatalf("err=%v", err)
	}
}
