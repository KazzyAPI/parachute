package backup

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type dumpEngine struct {
	fakeEngine
	result engine.ExecResult
}

func (d *dumpEngine) Exec(context.Context, string, []string, []string, time.Duration, int64) (engine.ExecResult, error) {
	return d.result, nil
}

type dumpLabels struct{}

func (dumpLabels) Value(_ domain.Container, name string) (string, bool) {
	return "echo dump", name == "backup.dump"
}

func TestRunDumpRefusesTruncatedOutput(t *testing.T) {
	fake := &dumpEngine{result: engine.ExecResult{Output: []byte("partial"), Truncated: true}}
	s := &Snapshotter{engine: fake, labels: dumpLabels{}}
	_, err := s.runDump(context.Background(), txn.Plan{}, storeRoot{}, "snap-1", domain.Container{Name: "a"}, engine.Spec{}, "stop")
	if err == nil || !strings.HasPrefix(err.Error(), reasonSnapshotFailed) {
		t.Fatalf("error %v", err)
	}
}

func TestRenamedOriginal(t *testing.T) {
	if got := renamedOriginal("web", "txn-20260101T000000Z-abcdef"); got != "web-parachute-abcdef" {
		t.Fatalf("got %s", got)
	}
}
