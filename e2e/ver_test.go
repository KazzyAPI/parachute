package e2e

import (
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestVER_001_process_exit_rolls_back(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "ver001")
	labels := map[string]string{"parachute.verify.stable": "5s"}
	ref := app.registry.PushSpec("ver001", "1", harness.ImageSpec{Verify: true, Version: "1"})
	container := env.RunContainer("ver001", ref, harness.ContainerOptions{Labels: labels})
	app.registry.PushSpec("ver001", "1", harness.ImageSpec{Verify: true, Version: "2", Env: map[string]string{"EXIT_AFTER": "2"}})
	result := armedRunOnce(env)
	assertRolledBack(t, result, container, "verify_failed")
}

func TestVER_002_health_succeeds_commits(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "ver002")
	health := []string{"CMD", "/fixture", "exit", "0"}
	ref := app.registry.PushSpec("ver002", "1", harness.ImageSpec{Verify: true, Version: "1", Healthcheck: health})
	container := env.RunContainer("ver002", ref, harness.ContainerOptions{})
	app.registry.PushSpec("ver002", "1", harness.ImageSpec{Verify: true, Version: "2", Healthcheck: health, Env: map[string]string{"FIXTURE_HEALTH": "starting"}})
	result := armedRunOnce(env)
	assertUpdated(t, result, container)
}

func TestVER_002_health_fails_rolls_back(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "ver002b")
	health := []string{"CMD", "/fixture", "exit", "1"}
	ref := app.registry.PushSpec("ver002b", "1", harness.ImageSpec{Verify: true, Version: "1"})
	container := env.RunContainer("ver002b", ref, harness.ContainerOptions{Labels: map[string]string{"parachute.verify.timeout": "30s", "parachute.verify.stable": "1s"}})
	app.registry.PushSpec("ver002b", "1", harness.ImageSpec{Verify: true, Version: "2", Healthcheck: health})
	result := armedRunOnce(env)
	assertRolledBack(t, result, container, "verify_failed")
}

func TestVER_003_stable_running_commits(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "ver003")
	ref := app.registry.PushSpec("ver003", "1", harness.ImageSpec{Verify: true, Version: "1"})
	container := env.RunContainer("ver003", ref, harness.ContainerOptions{Labels: map[string]string{"parachute.verify.stable": "5s"}})
	app.registry.PushSpec("ver003", "1", harness.ImageSpec{Verify: true, Version: "2"})
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
	ref := app.registry.PushSpec("ver003b", "1", harness.ImageSpec{Verify: true, Version: "1"})
	container := env.RunContainer("ver003b", ref, harness.ContainerOptions{
		Extra:  []string{"--restart", "always"},
		Labels: map[string]string{"parachute.verify.stable": "30s"},
	})
	app.registry.PushSpec("ver003b", "1", harness.ImageSpec{Verify: true, Version: "2", Env: map[string]string{"EXIT_AFTER": "10"}})
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
	spec := harness.ImageSpec{Verify: true, Version: "1", Env: map[string]string{"WRITE": "/ready", "WRITE_DATA": "ok"}}
	ref := app.registry.PushSpec("ver004", "1", spec)
	container := env.RunContainer("ver004", ref, harness.ContainerOptions{Labels: labels})
	delayed := harness.ImageSpec{Verify: true, Version: "2", Env: map[string]string{"WRITE": "/ready", "WRITE_DATA": "ok", "WRITE_AFTER": "10"}}
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
	ref := app.registry.PushSpec("ver004b", "1", harness.ImageSpec{Verify: true, Version: "1"})
	container := env.RunContainer("ver004b", ref, harness.ContainerOptions{Labels: labels})
	app.registry.PushSpec("ver004b", "1", harness.ImageSpec{Verify: true, Version: "2"})
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
	spec := harness.ImageSpec{Verify: true, Version: "1", Env: map[string]string{"HTTP": "8080"}}
	ref := app.registry.PushSpec("ver005", "1", spec)
	container := env.RunContainer("ver005", ref, harness.ContainerOptions{Labels: labels})
	app.registry.PushSpec("ver005", "1", harness.ImageSpec{Verify: true, Version: "2", Env: map[string]string{"HTTP": "8080"}})
	result := armedRunOnce(env)
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
	spec := harness.ImageSpec{Verify: true, Version: "1"}
	ref := app.registry.PushSpec("ver005b", "1", spec)
	container := env.RunContainer("ver005b", ref, harness.ContainerOptions{Labels: labels})
	bad := harness.ImageSpec{Verify: true, Version: "2", Env: map[string]string{"HTTP": "8080", "HTTP_CODE": "500"}}
	app.registry.PushSpec("ver005b", "1", bad)
	result := armedRunOnce(env)
	assertRolledBack(t, result, container, "verify_failed")
	assertReasonDetail(t, result, container, "500")
}

func TestVER_006_short_timeout_health_starting_rolls_back(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "ver006")
	ref := app.registry.PushSpec("ver006", "1", harness.ImageSpec{Verify: true, Version: "1", Healthcheck: []string{"CMD", "/fixture", "exit", "0"}})
	container := env.RunContainer("ver006", ref, harness.ContainerOptions{Labels: map[string]string{"parachute.verify.timeout": "20s"}})
	app.registry.PushSpec("ver006", "1", harness.ImageSpec{
		Verify: true, Version: "2", Healthcheck: []string{"CMD", "/fixture"}, HealthcheckInterval: "2s", HealthcheckTimeout: "1h",
		Env: map[string]string{"FIXTURE_HEALTH": "starting"},
	})
	env.ParachuteImage()
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
	health := []string{"CMD", "/fixture", "exit", "0"}
	ref := app.registry.PushSpec("ver006b", "1", harness.ImageSpec{Verify: true, Version: "1"})
	container := env.RunContainer("ver006b", ref, harness.ContainerOptions{})
	app.registry.PushSpec("ver006b", "1", harness.ImageSpec{Verify: true,
		Version: "2", Healthcheck: health,
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
	webRef := registry.PushSpec("web", "1", harness.ImageSpec{Verify: true, Version: "1"})
	apiRef := registry.PushSpec("api", "1", harness.ImageSpec{Verify: true, Version: "1"})
	web := env.RunContainer("web", webRef, harness.ContainerOptions{Labels: map[string]string{"com.docker.compose.project": "shop", "com.docker.compose.service": "web", "parachute.verify.stable": "2s"}})
	api := env.RunContainer("api", apiRef, harness.ContainerOptions{Labels: map[string]string{"com.docker.compose.project": "shop", "com.docker.compose.service": "api", "parachute.verify.stable": "2s"}})
	webID, webImage := env.ContainerState(web)
	apiID, apiImage := env.ContainerState(api)
	registry.PushSpec("web", "1", harness.ImageSpec{Verify: true, Version: "2"})
	registry.PushSpec("api", "1", harness.ImageSpec{Verify: true, Version: "2", Healthcheck: []string{"CMD", "/fixture", "exit", "1"}})
	result := armedRunOnce(env)
	assertRolledBack(t, result, web, "verify_failed")
	assertRolledBack(t, result, api, "verify_failed")
	gotWebID, gotWebImage := env.ContainerState(web)
	gotAPIID, gotAPIImage := env.ContainerState(api)
	if gotWebID != webID || gotWebImage != webImage || gotAPIID != apiID || gotAPIImage != apiImage {
		t.Fatalf("containers not restored: web %s/%s api %s/%s", gotWebID, gotWebImage, gotAPIID, gotAPIImage)
	}
}

func TestVER_012_no_restart_commits_without_health_wait(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "ver012")
	labels := map[string]string{"parachute.verify.stable": "30s"}
	health := []string{"CMD", "/fixture", "exit", "0"}
	ref := app.registry.PushSpec("ver012", "1", harness.ImageSpec{Verify: true, Version: "1", Healthcheck: health})
	container := env.RunContainer("ver012", ref, harness.ContainerOptions{Labels: labels})
	app.registry.PushSpec("ver012", "1", harness.ImageSpec{Verify: true, Version: "2", Healthcheck: health})
	result := armedRunOnce(env, "--no-restart")
	assertUpdated(t, result, container)
	if result.Duration > 10*time.Second {
		t.Fatalf("no-restart took %s", result.Duration)
	}
}
