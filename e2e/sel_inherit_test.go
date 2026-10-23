package e2e

import (
	"strings"
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestSEL_013_inherited_backup_label_is_ignored_and_warned(t *testing.T) {
	env := harness.NewEnv(t)
	app, publishNewer := labelledUpdatable(env, map[string]string{"parachute.backup": "none"})
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	publishNewer()
	result := runOnce(env)
	warnings := labelWarnings(result, a, "parachute.backup")
	if len(warnings) != 1 || !strings.Contains(warnings[0].Get("labels"), "parachute.backup") {
		t.Errorf("expected exactly one warn line naming %s and parachute.backup (the ignored label), got %d", a, len(warnings))
	}
}

func TestSEL_013_inherited_enable_false_is_ignored(t *testing.T) {
	env := harness.NewEnv(t)
	app, _ := labelledUpdatable(env, map[string]string{"parachute.enable": "false"})
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	result := runOnce(env, "--no-pull")
	assertCandidates(t, result, []string{a}, nil)
	if len(labelWarnings(result, a, "parachute.enable")) == 0 {
		t.Errorf("no warn line names %s and parachute.enable", a)
	}
}

func TestSEL_013_inherited_enable_true_does_not_opt_in(t *testing.T) {
	env := harness.NewEnv(t)
	app, _ := labelledUpdatable(env, map[string]string{"parachute.enable": "true"})
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	assertNoEntry(t, runOnce(env, "--no-pull", "--label-enable"), a)
}

func TestSEL_013_inherited_scope_does_not_select(t *testing.T) {
	env := harness.NewEnv(t)
	app, _ := labelledUpdatable(env, scoped(env))
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Unscoped: true})
	assertNoEntry(t, runOnce(env, "--no-pull"), a)
}

func TestSEL_013_inherited_monitor_only_is_honoured(t *testing.T) {
	env := harness.NewEnv(t)
	app, publishNewer := labelledUpdatable(env, map[string]string{"parachute.monitor-only": "true", "parachute.enable": "false"})
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	publishNewer()
	assertState(t, runOnce(env), a, "held", "monitor_only")
}

func TestSEL_013_operator_label_overrides_image_label(t *testing.T) {
	env := harness.NewEnv(t)
	app, _ := labelledUpdatable(env, map[string]string{"parachute.enable": "false"})
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Labels: map[string]string{"parachute.enable": "true"}})
	result := runOnce(env, "--no-pull")
	assertCandidates(t, result, []string{a}, nil)
	if warnings := labelWarnings(result, a, "parachute.enable"); len(warnings) > 0 {
		t.Errorf("unexpected warn line: %s", warnings[0].Raw)
	}
}

func TestSEL_013_out_of_scope_container_is_excluded_silently(t *testing.T) {
	env := harness.NewEnv(t)
	image := fixture(env)
	a := env.RunContainer("a", image, harness.ContainerOptions{Unscoped: true, Labels: map[string]string{"parachute.scope": env.Name("dev")}})
	_, imageID := env.ContainerState(a)
	proxy := env.ImageFaultProxy(imageID, image)
	result := env.Parachute("--run-once", "--porcelain", "v1", "--no-pull").Setenv("DOCKER_HOST", proxy.Endpoint()).Run(runTimeout)
	assertExit(t, result, 0)
	assertNoEntry(t, result, a)
	if warnings := findLogs(result.Logs(), func(entry harness.Log) bool {
		return entry.Level() == "warn" && strings.Contains(entry.Raw, a)
	}); len(warnings) > 0 {
		t.Errorf("unexpected warn line naming %s: %s", a, warnings[0].Raw)
	}
	for _, path := range proxy.ImageInspects() {
		if strings.Contains(path, imageID) || strings.Contains(path, image) {
			t.Errorf("image of %s was inspected: %s", a, path)
		}
	}
}

func TestSEL_013_image_inspect_failure_fails_candidate(t *testing.T) {
	env := harness.NewEnv(t)
	image := fixture(env)
	a := env.RunContainer("a", image, harness.ContainerOptions{})
	_, imageID := env.ContainerState(a)
	proxy := env.ImageFaultProxy(imageID, image)
	result := env.Parachute("--run-once", "--porcelain", "v1", "--no-pull").Setenv("DOCKER_HOST", proxy.Endpoint()).Run(runTimeout)
	assertState(t, result, a, "failed", "engine_error")
}
