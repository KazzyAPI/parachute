package verify

import (
	"context"
	"testing"
	"time"

	dockerspec "github.com/moby/docker-image-spec/specs-go/v1"
	"github.com/moby/moby/api/types/container"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type fakeClock struct {
	now time.Time
}

func (f *fakeClock) Now() time.Time { return f.now }

func (f *fakeClock) Sleep(ctx context.Context, duration time.Duration) error {
	f.now = f.now.Add(duration)
	return nil
}

type fakeInspector struct {
	spec    engine.Spec
	runtime engine.Runtime
	calls   int
}

func (f *fakeInspector) Inspect(context.Context, string) (engine.Spec, error) {
	return f.spec, nil
}

func (f *fakeInspector) Runtime(context.Context, string) (engine.Runtime, error) {
	f.calls++
	if f.calls > 1 && f.runtime.Health != nil && f.runtime.Health.Status == "starting" {
		f.runtime.Health.Status = "healthy"
	}
	return f.runtime, nil
}

type fakeCommander struct {
	exit int
}

func (f *fakeCommander) Exec(context.Context, string, []string, []string, time.Duration, int64) (engine.ExecResult, error) {
	return engine.ExecResult{ExitCode: f.exit}, nil
}

type fakeLabels struct {
	values map[string]string
}

func (f *fakeLabels) Duration(_ string, _ map[string]string, name string, fallback time.Duration) time.Duration {
	if value, ok := f.values[name]; ok {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func (f *fakeLabels) String(_ string, _ map[string]string, name string) (string, bool) {
	value, ok := f.values[name]
	return value, ok
}

func TestVerifier_created_container_passes(t *testing.T) {
	inspector := &fakeInspector{
		spec:    engine.Spec{Config: &container.Config{Labels: map[string]string{}}},
		runtime: engine.Runtime{Status: "created"},
	}
	verifier := New(inspector, nil, nil, &fakeLabels{}, Settings{Timeout: time.Minute, Stable: time.Second}, &fakeClock{now: time.Unix(0, 0)})
	verdict, err := verifier.Verify(context.Background(), txn.Plan{ID: "txn-20260101T000000Z-abcdef"}, "cid")
	if err != nil || !verdict.Passed {
		t.Fatalf("verdict %+v err %v", verdict, err)
	}
}

func TestVerifier_unhealthy_fails_immediately(t *testing.T) {
	inspector := &fakeInspector{
		spec: engine.Spec{Config: &container.Config{Healthcheck: &container.HealthConfig{Test: []string{"CMD", "false"}}}},
		runtime: engine.Runtime{
			Status:  "running",
			Running: true,
			Health:  &engine.Health{Status: "unhealthy", Log: "db connection refused"},
		},
	}
	verifier := New(inspector, nil, nil, &fakeLabels{}, Settings{Timeout: time.Minute, Stable: time.Second}, &fakeClock{now: time.Unix(0, 0)})
	verdict, err := verifier.Verify(context.Background(), txn.Plan{ID: "txn-20260101T000000Z-abcdef"}, "cid")
	if err != nil || verdict.Passed || verdict.Reason != ReasonFailed || verdict.Output != "db connection refused" {
		t.Fatalf("verdict %+v err %v", verdict, err)
	}
}

func TestVerifier_health_becomes_healthy(t *testing.T) {
	inspector := &fakeInspector{
		spec: engine.Spec{Config: &container.Config{Healthcheck: &container.HealthConfig{Test: []string{"CMD", "true"}}}},
		runtime: engine.Runtime{
			Status:  "running",
			Running: true,
			Health:  &engine.Health{Status: "starting"},
		},
	}
	verifier := New(inspector, nil, nil, &fakeLabels{}, Settings{Timeout: time.Minute, Stable: time.Second}, &fakeClock{now: time.Unix(0, 0)})
	verdict, err := verifier.Verify(context.Background(), txn.Plan{ID: "txn-20260101T000000Z-abcdef"}, "cid")
	if err != nil || !verdict.Passed {
		t.Fatalf("verdict %+v err %v", verdict, err)
	}
}

func TestVerifier_stable_window(t *testing.T) {
	start := time.Unix(0, 0)
	clock := &fakeClock{now: start}
	inspector := &fakeInspector{
		spec: engine.Spec{Config: &container.Config{}},
		runtime: engine.Runtime{
			Status:       "running",
			Running:      true,
			StartedAt:    start,
			RestartCount: 0,
		},
	}
	verifier := New(inspector, nil, nil, &fakeLabels{}, Settings{Timeout: time.Minute, Stable: 30 * time.Second}, clock)
	verdict, err := verifier.Verify(context.Background(), txn.Plan{ID: "txn-20260101T000000Z-abcdef"}, "cid")
	if err != nil || !verdict.Passed {
		t.Fatalf("verdict %+v err %v", verdict, err)
	}
}

func TestVerifier_cmd_must_exit_zero(t *testing.T) {
	start := time.Unix(0, 0)
	inspector := &fakeInspector{
		spec: engine.Spec{Config: &container.Config{}},
		runtime: engine.Runtime{
			Status: "running", Running: true, StartedAt: start, RestartCount: 0,
		},
	}
	labels := &fakeLabels{values: map[string]string{"verify.cmd": "exit 1", "verify.stable": "0s"}}
	verifier := New(inspector, &fakeCommander{exit: 1}, nil, labels, Settings{Timeout: time.Minute, Stable: 0}, &fakeClock{now: start})
	verdict, err := verifier.Verify(context.Background(), txn.Plan{ID: "txn-20260101T000000Z-abcdef"}, "cid")
	if err != nil || verdict.Passed || verdict.Reason != ReasonFailed {
		t.Fatalf("verdict %+v err %v", verdict, err)
	}
}

func TestVerifier_base_only_skips_cmd(t *testing.T) {
	start := time.Unix(0, 0)
	inspector := &fakeInspector{
		spec: engine.Spec{Config: &container.Config{}},
		runtime: engine.Runtime{
			Status: "running", Running: true, StartedAt: start,
		},
	}
	labels := &fakeLabels{values: map[string]string{"verify.cmd": "exit 1", "verify.stable": "0s"}}
	verifier := New(inspector, &fakeCommander{exit: 1}, nil, labels, Settings{Timeout: time.Minute, Stable: 0}, &fakeClock{now: start})
	verdict, err := verifier.VerifyBase(context.Background(), txn.Plan{ID: "txn-20260101T000000Z-abcdef"}, "cid")
	if err != nil || !verdict.Passed {
		t.Fatalf("verdict %+v err %v", verdict, err)
	}
}

func TestOperatorLabels_drops_labels_inherited_from_the_image(t *testing.T) {
	spec := engine.Spec{
		Config: &container.Config{Labels: map[string]string{
			"parachute.verify.cmd": "exit 1", "parachute.verify.url": "http://x", "other": "same",
		}},
		ImageConfig: &dockerspec.DockerOCIImageConfig{ImageConfig: ocispec.ImageConfig{Labels: map[string]string{
			"parachute.verify.cmd": "exit 1", "parachute.verify.url": "http://y", "other": "same",
		}}},
	}
	got := operatorLabels(spec)
	if _, ok := got["parachute.verify.cmd"]; ok {
		t.Fatalf("inherited label kept: %v", got)
	}
	if got["parachute.verify.url"] != "http://x" || got["other"] != "same" {
		t.Fatalf("labels %v", got)
	}
}

func TestVER_010_rollback_verification_does_not_wait_the_stable_window(t *testing.T) {
	start := time.Unix(0, 0)
	clock := &fakeClock{now: start}
	inspector := &fakeInspector{
		spec:    engine.Spec{Config: &container.Config{}},
		runtime: engine.Runtime{Status: "running", Running: true, StartedAt: start},
	}
	verifier := New(inspector, nil, nil, &fakeLabels{}, Settings{Timeout: time.Minute, Stable: 30 * time.Second}, clock)
	verdict, err := verifier.VerifyBase(context.Background(), txn.Plan{ID: "txn-20260101T000000Z-abcdef"}, "cid")
	if err != nil || !verdict.Passed || clock.now.Sub(start) != 0 {
		t.Fatalf("verdict %+v err %v waited %s", verdict, err, clock.now.Sub(start))
	}
}

func TestVerifier_redacts_container_env_values_in_output(t *testing.T) {
	inspector := &fakeInspector{
		spec: engine.Spec{Config: &container.Config{
			Env:         []string{"DB_PASSWORD=pw-9z7"},
			Healthcheck: &container.HealthConfig{Test: []string{"CMD", "false"}},
		}},
		runtime: engine.Runtime{
			Status:  "running",
			Running: true,
			Health:  &engine.Health{Status: "unhealthy", Log: "login failed with pw-9z7"},
		},
	}
	verifier := New(inspector, nil, nil, &fakeLabels{}, Settings{Timeout: time.Minute, Stable: time.Second}, &fakeClock{now: time.Unix(0, 0)})
	verdict, err := verifier.Verify(context.Background(), txn.Plan{ID: "txn-20260101T000000Z-abcdef"}, "cid")
	if err != nil || verdict.Output != "login failed with [REDACTED]" {
		t.Fatalf("verdict %+v err %v", verdict, err)
	}
}
