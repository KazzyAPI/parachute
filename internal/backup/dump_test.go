package backup

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/txn"
	"github.com/moby/moby/api/types/container"
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

type listingEngine struct {
	fakeEngine
}

func (listingEngine) Containers(context.Context) ([]domain.Container, error) {
	return []domain.Container{{ID: "id", Name: "a"}}, nil
}

func (listingEngine) Inspect(context.Context, string) (engine.Spec, error) { return engine.Spec{}, nil }

func TestTakeReturnsNoSnapshotWhenEveryTargetIsUnbacked(t *testing.T) {
	s := &Snapshotter{engine: &listingEngine{}, ownBackup: func(domain.Container) string { return "none" }}
	plan := txn.Plan{Targets: []txn.Target{{Container: "a"}}}
	id, err := s.Take(context.Background(), plan)
	if err != nil || id != NoSnapshot {
		t.Fatalf("id %q err %v", id, err)
	}
}

func TestRunDumpLogsStderrWithoutContainerEnvValues(t *testing.T) {
	var logged strings.Builder
	fake := &dumpEngine{result: engine.ExecResult{ExitCode: 1, Output: []byte("s3cr3t-pw\n")}}
	s := &Snapshotter{engine: fake, labels: dumpLabels{}, logger: slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelDebug}))}
	spec := engine.Spec{Config: &container.Config{Env: []string{"MYSQL_ROOT_PASSWORD=s3cr3t-pw"}}}
	if _, err := s.runDump(context.Background(), txn.Plan{}, storeRoot{}, "snap-1", domain.Container{Name: "a"}, spec, "stop"); err == nil {
		t.Fatal("want dump failure")
	}
	if strings.Contains(logged.String(), "s3cr3t-pw") || !strings.Contains(logged.String(), "[REDACTED]") {
		t.Fatalf("logged %s", logged.String())
	}
}
