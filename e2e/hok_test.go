package e2e

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestHOK_001_disabled_hook_does_not_run(t *testing.T) {
	env := harness.NewEnv(t)
	hostDir, mount := hookVolume(env)
	mark := hookFile(mount)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{
		Extra:  hookContainerExtra(hostDir, mount),
		Labels: hookLabel("pre-update", "touch "+mark),
	})
	app.publishNewer()
	result := runOnce(env, "--armed", "--delay-days", "0", "--protect-databases=false", "--helper-image", env.ParachuteImage())
	assertState(t, result, a, "updated", "")
	assertHookFile(t, filepath.Join(hostDir, "ran"), false)
}

func TestHOK_001_enabled_hook_runs(t *testing.T) {
	env := harness.NewEnv(t)
	hostDir, mount := hookVolume(env)
	mark := hookFile(mount)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{
		Extra:  hookContainerExtra(hostDir, mount),
		Labels: hookLabel("pre-update", "touch "+mark),
	})
	app.publishNewer()
	result := runOnce(env, armedHooks(env)...)
	assertState(t, result, a, "updated", "")
	assertHookFile(t, filepath.Join(hostDir, "ran"), true)
}

func TestHOK_002_stopped_candidate_hook_skipped_and_still_checked(t *testing.T) {
	env := harness.NewEnv(t)
	hostDir, mount := hookVolume(env)
	mark := hookFile(mount)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{
		Extra:  hookContainerExtra(hostDir, mount),
		Labels: hookLabel("pre-check", "touch "+mark),
	})
	env.Docker("stop", a)
	app.publishNewer()
	result := runOnce(env, append(armedHooks(env), "--include-stopped")...)
	assertState(t, result, a, "would_update", "")
	assertHookFile(t, filepath.Join(hostDir, "ran"), false)
	skipped := findLogs(result.Logs(), func(entry harness.Log) bool {
		return entry.Level() == "debug" && entry.Get("hook") == "pre-check" && entry.Msg() == "hook skipped"
	})
	if len(skipped) == 0 {
		t.Fatal("expected debug skip log for pre-check")
	}
}

func TestHOK_005_pre_check_timeout_continues_after_60s_with_warn(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{
		Labels: hookLabel("pre-check", "sleep 90"),
	})
	app.publishNewer()
	started := time.Now()
	result := runOnce(env, armedHooks(env)...)
	if result.Duration < 55*time.Second || result.Duration > 75*time.Second {
		t.Fatalf("expected cycle near 60s, took %s", result.Duration)
	}
	if time.Since(started) < 55*time.Second {
		t.Fatalf("finished too quickly: %s", result.Duration)
	}
	warns := findLogs(result.Logs(), func(entry harness.Log) bool {
		return entry.Level() == "warn" && entry.Get("hook") == "pre-check"
	})
	if len(warns) == 0 {
		t.Fatal("expected warn log naming pre-check")
	}
	assertState(t, result, a, "would_update", "")
}

func TestHOK_005_pre_update_timeout_label_allows_long_hook(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{
		Labels: map[string]string{
			"parachute.lifecycle.pre-update":         "sleep 90",
			"parachute.lifecycle.pre-update-timeout": "2",
		},
	})
	app.publishNewer()
	result := runOnce(env, armedHooks(env)...)
	if result.Duration < 85*time.Second {
		t.Fatalf("expected hook to run ~90s, took %s", result.Duration)
	}
	assertState(t, result, a, "updated", "")
}

func TestHOK_005_zero_timeout_bounded_by_transaction_ceiling(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{
		Labels: map[string]string{
			"parachute.lifecycle.pre-update":         "sleep 3600",
			"parachute.lifecycle.pre-update-timeout": "0",
		},
	})
	id, image := env.ContainerState(a)
	app.publishNewer()
	result := env.Parachute(append(armedHooks(env, "--transaction-timeout", "2m"), "--porcelain", "v1", "--run-once")...).Run(3 * time.Minute)
	if result.Duration < 110*time.Second || result.Duration > 150*time.Second {
		t.Fatalf("expected failure near 2m, took %s", result.Duration)
	}
	assertState(t, result, a, "failed", "transaction_timeout")
	assertUnchanged(env, a, id, image)
	ceiling := findLogs(result.Logs(), func(entry harness.Log) bool {
		return entry.Level() == "warn" && entry.Get("hook") == "pre-update"
	})
	if len(ceiling) == 0 {
		t.Fatal("expected warn naming pre-update at ceiling")
	}
}

func TestHOK_006_post_update_failure_commits_and_warns(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{
		Labels: hookLabel("post-update", "exit 1"),
	})
	app.publishNewer()
	result := runOnce(env, armedHooks(env)...)
	assertState(t, result, a, "updated", "")
	warns := findLogs(result.Logs(), func(entry harness.Log) bool {
		return entry.Level() == "warn" && entry.Get("hook") == "post-update" && entry.Get("exit_code") == "1"
	})
	if len(warns) == 0 {
		t.Fatal("expected warn for post-update exit 1")
	}
}

func TestHOK_006_post_check_75_is_silent_success(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{
		Labels: hookLabel("post-check", "exit 75"),
	})
	app.publishNewer()
	result := runOnce(env, armedHooks(env)...)
	assertState(t, result, a, "would_update", "")
	for _, entry := range result.Logs() {
		if entry.Level() == "warn" && strings.Contains(entry.Msg(), "hook") {
			t.Fatalf("unexpected hook warn: %s", entry.Raw)
		}
	}
}

func TestHOK_008_secret_not_in_log_report_or_webhook(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{
		Extra:  []string{"-e", "API_KEY=hunter2-secret"},
		Labels: hookLabel("post-update", `echo "$API_KEY"; exit 1`),
	})
	app.publishNewer()
	result := notify(env, webhook, append(armedHooks(env, "--log-level", "debug"), "--notification-report=false")...)
	assertState(t, result, a, "updated", "")
	secret := "hunter2-secret"
	assertNoSecrets(t, "stderr", result.Stderr, secret)
	assertNoSecrets(t, "stdout", result.Stdout, secret)
	assertNotificationsHaveNoSecrets(t, webhook, secret)
	assertContains(t, result.Stderr, "[REDACTED]")
}

func TestHOK_001_enable_lifecycle_hooks_is_not_startup_error(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once", "--enable-lifecycle-hooks").Run(runTimeout)
	assertExit(t, result, 0)
}

func TestHOK_001_environment_variable_enables_hooks(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once").Setenv("PARACHUTE_ENABLE_LIFECYCLE_HOOKS", "true").Run(runTimeout)
	assertExit(t, result, 0)
}
