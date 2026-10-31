package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
)

type flowEngine struct {
	fakeEngine
	containers []domain.Container
	inspectErr error
	startErr   error
	started    []string
}

func (f *flowEngine) Containers(context.Context) ([]domain.Container, error) {
	return f.containers, nil
}

func (f *flowEngine) Inspect(context.Context, string) (engine.Spec, error) {
	return engine.Spec{Mounts: []container.MountPoint{{Type: mount.TypeVolume, Name: "shared"}}}, f.inspectErr
}

func (f *flowEngine) Start(_ context.Context, id string) error {
	f.started = append(f.started, id)
	return f.startErr
}

func TestOutsiderVolumesReturnsInspectErrorButSkipsVanishedContainers(t *testing.T) {
	running := []domain.Container{{ID: "out", Name: "out", State: "running"}}
	if _, err := outsiderVolumes(context.Background(), &flowEngine{containers: running, inspectErr: errors.New("boom")}, nil); err == nil {
		t.Fatal("expected the inspect error")
	}
	gone := &flowEngine{containers: running, inspectErr: fmt.Errorf("gone: %w", cerrdefs.ErrNotFound)}
	if outsiders, err := outsiderVolumes(context.Background(), gone, nil); err != nil || len(outsiders) != 0 {
		t.Fatalf("outsiders %v err %v", outsiders, err)
	}
}

func TestRestartStoppedLogsFailuresAndTracksStopBeforeStopReturns(t *testing.T) {
	var log bytes.Buffer
	fake := &flowEngine{startErr: errors.New("no start")}
	s := &Snapshotter{engine: fake, logger: slog.New(slog.NewTextHandler(&log, nil)), stopped: []string{"c1"}}
	s.restartStopped(context.Background())
	if len(fake.started) != 1 || !strings.Contains(log.String(), "cannot restart") {
		t.Fatalf("started %v log %q", fake.started, log.String())
	}
}

func TestRunnerLabelsRestoreVerifyHelpersSeparatelyFromTransactions(t *testing.T) {
	r := &runner{restoreVerify: "snap-1"}
	labels := r.spec(nil, nil).Labels
	if labels["parachute.restore-verify"] != "snap-1" || labels["parachute.transaction"] != "" {
		t.Fatalf("labels %v", labels)
	}
}
