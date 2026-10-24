package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestVER_001_process_exit_rolls_back(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "ver001")
	labels := map[string]string{"parachute.verify.stable": "2s"}
	spec := harness.ImageSpec{Verify: true, Version: "1", Labels: labels, Env: map[string]string{"EXIT_AFTER": "2"}}
	ref := env.Registry().PushSpec("ver001", "1", spec)
	container := env.RunContainer("ver001", ref, harness.ContainerOptions{})
	app.registry.PushSpec("ver001", "1", harness.ImageSpec{Verify: true, Version: "2", Labels: labels, Env: map[string]string{"EXIT_AFTER": "2"}})
	result := armedRunOnce(env)
	assertRolledBack(t, result, container, "verify_failed")
}

func TestVER_002_health_succeeds_commits(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "ver002")
	labels := map[string]string{"parachute.verify.stable": "1s"}
	health := []string{"CMD", "/fixture", "exit", "0"}
	ref := env.Registry().PushSpec("ver002", "1", harness.ImageSpec{Verify: true, Version: "1", Labels: labels, Healthcheck: health})
	container := env.RunContainer("ver002", ref, harness.ContainerOptions{})
	app.registry.PushSpec("ver002", "1", harness.ImageSpec{Verify: true, Version: "2", Labels: labels, Healthcheck: health, Env: map[string]string{"FIXTURE_HEALTH": "starting"}})
	result := armedRunOnce(env)
	assertUpdated(t, result, container)
}

func TestVER_002_health_fails_rolls_back(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "ver002b")
	labels := map[string]string{"parachute.verify.timeout": "30s"}
	health := []string{"CMD", "/fixture", "exit", "1"}
	ref := env.Registry().PushSpec("ver002b", "1", harness.ImageSpec{Verify: true, Version: "1", Labels: labels})
	container := env.RunContainer("ver002b", ref, harness.ContainerOptions{})
	app.registry.PushSpec("ver002b", "1", harness.ImageSpec{Verify: true, Version: "2", Labels: labels, Healthcheck: health})
	result := armedRunOnce(env)
	assertRolledBack(t, result, container, "verify_failed")
}

func TestVER_003_stable_running_commits(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "ver003")
	labels := map[string]string{"parachute.verify.stable": "5s"}
	ref := env.Registry().PushSpec("ver003", "1", harness.ImageSpec{Verify: true, Version: "1", Labels: labels})
	container := env.RunContainer("ver003", ref, harness.ContainerOptions{})
	app.registry.PushSpec("ver003", "1", harness.ImageSpec{Verify: true, Version: "2", Labels: labels})
	start := time.Now()
	result := armedRunOnce(env)
	assertUpdated(t, result, container)
	if elapsed := time.Since(start); elapsed < 5*time.Second || elapsed > 15*time.Second {
		t.Fatalf("commit timing %s", elapsed)
	}
}

func TestVER_003_exit_with_restart_rolls_back(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "ver003b")
	labels := map[string]string{"parachute.verify.stable": "30s"}
	spec := harness.ImageSpec{Verify: true, Version: "1", Labels: labels, Env: map[string]string{"EXIT_AFTER": "10"}}
	ref := env.Registry().PushSpec("ver003b", "1", spec)
	container := env.RunContainer("ver003b", ref, harness.ContainerOptions{Extra: []string{"--restart", "always"}})
	app.registry.PushSpec("ver003b", "1", spec)
	result := armedRunOnce(env)
	assertRolledBack(t, result, container, "verify_failed")
}

func TestVER_004_cmd_ready_commits(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "ver004")
	labels := map[string]string{
		"parachute.verify.stable": "1s",
		"parachute.verify.cmd":    "test -f /ready",
	}
	spec := harness.ImageSpec{Verify: true, Version: "1", Labels: labels, Env: map[string]string{"WRITE": "/ready", "WRITE_DATA": "ok"}}
	ref := env.Registry().PushSpec("ver004", "1", spec)
	container := env.RunContainer("ver004", ref, harness.ContainerOptions{})
	delayed := harness.ImageSpec{Verify: true, Version: "2", Labels: labels, Env: map[string]string{"WRITE": "/ready", "WRITE_DATA": "ok", "WRITE_AFTER": "10"}}
	app.registry.PushSpec("ver004", "1", delayed)
	result := armedRunOnce(env)
	assertUpdated(t, result, container)
}

func TestVER_004_cmd_exit1_rolls_back(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "ver004b")
	labels := map[string]string{
		"parachute.verify.stable":  "1s",
		"parachute.verify.timeout": "10s",
		"parachute.verify.cmd":     "exit 1",
	}
	ref := env.Registry().PushSpec("ver004b", "1", harness.ImageSpec{Verify: true, Version: "1", Labels: labels})
	container := env.RunContainer("ver004b", ref, harness.ContainerOptions{})
	app.registry.PushSpec("ver004b", "1", harness.ImageSpec{Verify: true, Version: "2", Labels: labels})
	result := armedRunOnce(env)
	assertRolledBack(t, result, container, "verify_failed")
}

func TestVER_005_url_200_commits(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "ver005")
	labels := map[string]string{
		"parachute.verify.stable": "1s",
		"parachute.verify.url":    "http://localhost:8080/health",
	}
	spec := harness.ImageSpec{Verify: true, Version: "1", Labels: labels, Env: map[string]string{"HTTP": "8080"}}
	ref := env.Registry().PushSpec("ver005", "1", spec)
	container := env.RunContainer("ver005", ref, harness.ContainerOptions{})
	app.registry.PushSpec("ver005", "1", harness.ImageSpec{Verify: true, Version: "2", Labels: labels, Env: map[string]string{"HTTP": "8080"}})
	result := armedRunOnce(env, "--helper-image", env.ParachuteImage())
	assertUpdated(t, result, container)
}

func TestVER_005_url_500_rolls_back(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "ver005b")
	labels := map[string]string{
		"parachute.verify.stable":  "1s",
		"parachute.verify.timeout": "10s",
		"parachute.verify.url":     "http://localhost:8080/health",
	}
	spec := harness.ImageSpec{Verify: true, Version: "1", Labels: labels}
	ref := env.Registry().PushSpec("ver005b", "1", spec)
	container := env.RunContainer("ver005b", ref, harness.ContainerOptions{})
	bad := harness.ImageSpec{Verify: true, Version: "2", Labels: labels, Env: map[string]string{"HTTP": "8080", "HTTP_CODE": "500"}}
	app.registry.PushSpec("ver005b", "1", bad)
	result := armedRunOnce(env, "--helper-image", env.ParachuteImage())
	assertRolledBack(t, result, container, "verify_failed")
	entry := result.Entry(container)
	if entry == nil {
		t.Fatalf("no report entry for %s", container)
	}
	detail, _ := entry["detail"].(string)
	if !strings.Contains(detail, "500") {
		t.Fatalf("detail %q", detail)
	}
}

func TestVER_006_short_timeout_health_starting_rolls_back(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "ver006")
	labels := map[string]string{"parachute.verify.timeout": "20s"}
	health := []string{"CMD", "/fixture", "exit", "0"}
	ref := env.Registry().PushSpec("ver006", "1", harness.ImageSpec{Verify: true, Version: "1", Labels: labels, Healthcheck: []string{"CMD", "/fixture", "exit", "0"}})
	container := env.RunContainer("ver006", ref, harness.ContainerOptions{})
	app.registry.PushSpec("ver006", "1", harness.ImageSpec{Verify: true, Version: "2", Labels: labels, Healthcheck: health, Env: map[string]string{"FIXTURE_HEALTH": "starting"}})
	start := time.Now()
	result := armedRunOnce(env)
	assertRolledBack(t, result, container, "verify_timeout")
	if elapsed := time.Since(start); elapsed < 20*time.Second || elapsed > 30*time.Second {
		t.Fatalf("timeout timing %s", elapsed)
	}
}

func TestVER_006_long_start_period_commits(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "ver006b")
	labels := map[string]string{}
	health := []string{"CMD", "/fixture", "exit", "0"}
	ref := env.Registry().PushSpec("ver006b", "1", harness.ImageSpec{Verify: true, Version: "1", Labels: labels})
	container := env.RunContainer("ver006b", ref, harness.ContainerOptions{})
	app.registry.PushSpec("ver006b", "1", harness.ImageSpec{Verify: true,
		Version: "2", Labels: labels, Healthcheck: health,
		HealthcheckStartPeriod: "200s", HealthcheckInterval: "30s",
		Env: map[string]string{"FIXTURE_HEALTH": "starting"},
	})
	result := armedRunOnce(env)
	assertUpdated(t, result, container)
}

func TestVER_006_default_120s_timeout_skipped_needs_two_minute_real_wait(t *testing.T) {
	t.Skip()
}

func TestVER_006_deadline_after_post_update_hook_skipped_until_HOK_002_hooks_exist(t *testing.T) {
	t.Skip()
}

func TestVER_007_stack_partial_fail_rolls_back_both(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	webRef := registry.PushSpec("web", "1", harness.ImageSpec{Verify: true, Version: "1", Labels: map[string]string{"parachute.verify.stable": "2s"}})
	apiRef := registry.PushSpec("api", "1", harness.ImageSpec{Verify: true, Version: "1", Labels: map[string]string{"parachute.verify.stable": "2s"}})
	webID, webImage := env.ContainerState(env.RunContainer("web", webRef, harness.ContainerOptions{Labels: map[string]string{"com.docker.compose.project": "shop", "com.docker.compose.service": "web"}}))
	apiID, apiImage := env.ContainerState(env.RunContainer("api", apiRef, harness.ContainerOptions{Labels: map[string]string{"com.docker.compose.project": "shop", "com.docker.compose.service": "api"}}))
	registry.PushSpec("web", "1", harness.ImageSpec{Verify: true, Version: "2", Labels: map[string]string{"parachute.verify.stable": "2s"}})
	registry.PushSpec("api", "1", harness.ImageSpec{Verify: true, Version: "2", Labels: map[string]string{"parachute.verify.stable": "2s"}, Healthcheck: []string{"CMD", "/fixture", "exit", "1"}})
	result := armedRunOnce(env)
	assertRolledBack(t, result, "web", "verify_failed")
	assertRolledBack(t, result, "api", "verify_failed")
	gotWebID, gotWebImage := env.ContainerState("web")
	gotAPIID, gotAPIImage := env.ContainerState("api")
	if gotWebID != webID || gotWebImage != webImage || gotAPIID != apiID || gotAPIImage != apiImage {
		t.Fatalf("containers not restored: web %s/%s api %s/%s", gotWebID, gotWebImage, gotAPIID, gotAPIImage)
	}
}

func TestVER_012_no_restart_commits_without_health_wait(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "ver012")
	labels := map[string]string{"parachute.verify.stable": "30s"}
	health := []string{"CMD", "/fixture", "exit", "0"}
	ref := env.Registry().PushSpec("ver012", "1", harness.ImageSpec{Verify: true, Version: "1", Labels: labels, Healthcheck: health})
	container := env.RunContainer("ver012", ref, harness.ContainerOptions{})
	app.registry.PushSpec("ver012", "1", harness.ImageSpec{Verify: true, Version: "2", Labels: labels, Healthcheck: health})
	start := time.Now()
	result := armedRunOnce(env, "--no-restart")
	assertUpdated(t, result, container)
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("no-restart took %s", elapsed)
	}
}
