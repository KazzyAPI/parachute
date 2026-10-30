package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestHOK_006_deferred_update_proceeds_next_cycle(t *testing.T) {
	env := harness.NewEnv(t)
	hostDir, mount := hookVolume(env)
	app := newShellUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{
		Extra:  hookContainerExtra(hostDir, mount),
		Labels: hookLabel("pre-update", "test -f "+filepath.Join(mount, "go")+" || exit 75"),
	})
	app.publishNewer()
	deferred := runOnce(env, armedHooks(env)...)
	assertState(t, deferred, a, "held", "hook_deferred")
	if err := os.WriteFile(filepath.Join(hostDir, "go"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	app.publishNewer()
	result := runOnce(env, armedHooks(env)...)
	assertState(t, result, a, "updated", "")
}

func TestHOK_006_stack_deferral_holds_both_and_no_die_event(t *testing.T) {
	env := harness.NewEnv(t)
	rec := env.StartRawEventRecorder()
	hostDir, mount := hookVolume(env)
	mark := filepath.Join(mount, "order")
	dbMark := filepath.Join(mount, "db-order")
	stack := env.Name("shop")
	appImage := pushShell(env.Registry(), "app", "1")
	dbImage := pushShell(env.Registry(), "db", "1")
	appLabels := mergeLabels(composeLabels(stack, "app"), hookOrderLabels(mark))
	appLabels["parachute.lifecycle.pre-update"] = "exit 75"
	app := env.RunContainer("app", appImage, harness.ContainerOptions{
		Extra:  hookContainerExtra(hostDir, mount),
		Labels: appLabels,
	})
	db := env.RunContainer("db", dbImage, harness.ContainerOptions{
		Extra:  hookContainerExtra(hostDir, mount),
		Labels: mergeLabels(composeLabels(stack, "db"), hookOrderLabels(dbMark)),
	})
	pushShell(env.Registry(), "app", "2")
	pushShell(env.Registry(), "db", "2")
	result := runOnce(env, armedHooks(env)...)
	assertState(t, result, app, "held", "hook_deferred")
	assertState(t, result, db, "held", "hook_deferred")
	for _, line := range rec.Events() {
		if strings.Contains(line, " die ") && (strings.Contains(line, "app") || strings.Contains(line, "db")) {
			t.Fatalf("unexpected die event: %s", line)
		}
	}
	if got := readHookOrder(t, filepath.Join(hostDir, "order")); strings.Contains(got, "post-update") {
		t.Fatalf("app hook order %q", got)
	}
	if got := readHookOrder(t, filepath.Join(hostDir, "db-order")); strings.Contains(got, "post-update") {
		t.Fatalf("db hook order %q", got)
	}
}

func TestHOK_007_failing_pre_update_aborts_before_stop(t *testing.T) {
	env := harness.NewEnv(t)
	app := newShellUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{
		Labels: hookLabel("pre-update", "exit 1"),
	})
	id, image := env.ContainerState(a)
	app.publishNewer()
	result := runOnce(env, armedHooks(env)...)
	assertState(t, result, a, "failed", "pre_update_hook_failed")
	assertUnchanged(env, a, id, image)
}

func TestPOL_001_hook_deferred_is_last_hold_reason(t *testing.T) {
	env := harness.NewEnv(t)
	app := newShellUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{
		Labels: hookLabel("pre-update", "exit 75"),
	})
	app.publishNewer()
	result := runOnce(env, armedHooks(env)...)
	assertState(t, result, a, "held", "hook_deferred")
}

func TestUPD_019_pre_update_hook_without_timeout_hits_ceiling(t *testing.T) {
	env := harness.NewEnv(t)
	app := newShellUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{
		Labels: map[string]string{
			"parachute.lifecycle.pre-update":         "sleep 3600",
			"parachute.lifecycle.pre-update-timeout": "0",
		},
	})
	id, image := env.ContainerState(a)
	app.publishNewer()
	result := env.Parachute(append(armedHooks(env, "--transaction-timeout", "2m"), "--porcelain", "v1", "--run-once")...).Run(3 * time.Minute)
	assertState(t, result, a, "failed", "transaction_timeout")
	assertUnchanged(env, a, id, image)
}

func mergeLabels(parts ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, part := range parts {
		for key, value := range part {
			out[key] = value
		}
	}
	return out
}
