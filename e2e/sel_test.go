package e2e

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func fixture(env *harness.Env) string {
	env.T.Helper()
	return env.Registry().Push("app", "1", "1")
}

func TestSEL_001_default_selects_all_running(t *testing.T) {
	env := harness.NewEnv(t)
	image := fixture(env)
	names := []string{
		env.RunContainer("a", image, harness.ContainerOptions{Unscoped: true}),
		env.RunContainer("b", image, harness.ContainerOptions{Unscoped: true}),
		env.RunContainer("c", image, harness.ContainerOptions{Unscoped: true}),
	}
	result := env.Parachute("--run-once", "--porcelain", "v1", "--no-pull").Unscoped().Run(runTimeout)
	assertExit(t, result, 0)
	assertCandidates(t, result, names, nil)
}

func TestSEL_001_exited_container_is_not_a_candidate(t *testing.T) {
	env := harness.NewEnv(t)
	image := fixture(env)
	running := env.RunContainer("running", image, harness.ContainerOptions{Unscoped: true})
	exited := env.RunContainer("exited", image, harness.ContainerOptions{Unscoped: true, Args: []string{"exit", "0"}})
	env.Docker("wait", exited)
	result := env.Parachute("--run-once", "--porcelain", "v1", "--no-pull").Unscoped().Run(runTimeout)
	assertCandidates(t, result, []string{running}, []string{exited})
}

func TestSEL_002_positional_names(t *testing.T) {
	env := harness.NewEnv(t)
	image := fixture(env)
	a := env.RunContainer("a", image, harness.ContainerOptions{})
	b := env.RunContainer("b", image, harness.ContainerOptions{})
	c := env.RunContainer("c", image, harness.ContainerOptions{})
	result := runOnce(env, a, b)
	assertCandidates(t, result, []string{a, b}, []string{c})
}

func TestSEL_002_repeated_name_flag(t *testing.T) {
	env := harness.NewEnv(t)
	image := fixture(env)
	a := env.RunContainer("a", image, harness.ContainerOptions{})
	b := env.RunContainer("b", image, harness.ContainerOptions{})
	c := env.RunContainer("c", image, harness.ContainerOptions{})
	result := runOnce(env, "--name", a, "--name", c)
	assertCandidates(t, result, []string{a, c}, []string{b})
}

func TestSEL_002_unmatched_name_is_warned(t *testing.T) {
	env := harness.NewEnv(t)
	missing := env.Name("zz")
	result := runOnce(env, missing)
	warnings := findLogs(result.Logs(), func(entry harness.Log) bool { return entry.Level() == "warn" && entry.Get("name") == missing })
	if len(warnings) != 1 {
		t.Fatalf("expected one warn line naming %s, got %d", missing, len(warnings))
	}
}

func TestSEL_003_opt_out_label(t *testing.T) {
	env := harness.NewEnv(t)
	image := fixture(env)
	a := env.RunContainer("a", image, harness.ContainerOptions{Labels: map[string]string{"parachute.enable": "false"}})
	b := env.RunContainer("b", image, harness.ContainerOptions{})
	result := runOnce(env)
	assertCandidates(t, result, []string{b}, []string{a})
	if logs := findLogs(result.Logs(), func(entry harness.Log) bool { return entry.Get("container") == a }); len(logs) > 0 {
		t.Errorf("container %s was checked: %v", a, logs[0].Raw)
	}
}

func TestSEL_004_opt_in_mode(t *testing.T) {
	env := harness.NewEnv(t)
	image := fixture(env)
	a := env.RunContainer("a", image, harness.ContainerOptions{Labels: map[string]string{"parachute.enable": "true"}})
	b := env.RunContainer("b", image, harness.ContainerOptions{})
	result := runOnce(env, "--label-enable")
	assertCandidates(t, result, []string{a}, []string{b})
}

func TestSEL_005_exclude_list_beats_enable_label(t *testing.T) {
	env := harness.NewEnv(t)
	image := fixture(env)
	a := env.RunContainer("a", image, harness.ContainerOptions{Labels: map[string]string{"parachute.enable": "true"}})
	result := runOnce(env, "--disable-containers", a)
	assertCandidates(t, result, nil, []string{a})
}

func TestSEL_005_exclude_list_from_environment(t *testing.T) {
	env := harness.NewEnv(t)
	image := fixture(env)
	a := env.RunContainer("a", image, harness.ContainerOptions{})
	b := env.RunContainer("b", image, harness.ContainerOptions{})
	c := env.RunContainer("c", image, harness.ContainerOptions{})
	result := env.Parachute("--run-once", "--porcelain", "v1").Setenv("PARACHUTE_DISABLE_CONTAINERS", a+","+b).Run(runTimeout)
	assertCandidates(t, result, []string{c}, []string{a, b})
}

func TestSEL_006_name_and_opt_out_combine(t *testing.T) {
	env := harness.NewEnv(t)
	image := fixture(env)
	a := env.RunContainer("a", image, harness.ContainerOptions{Labels: map[string]string{"parachute.enable": "false"}})
	result := runOnce(env, a)
	assertCandidates(t, result, nil, []string{a})
}

func TestSEL_006_opt_in_and_name_combine(t *testing.T) {
	env := harness.NewEnv(t)
	image := fixture(env)
	a := env.RunContainer("a", image, harness.ContainerOptions{})
	b := env.RunContainer("b", image, harness.ContainerOptions{Labels: map[string]string{"parachute.enable": "true"}})
	result := runOnce(env, "--label-enable", a)
	assertCandidates(t, result, nil, []string{a, b})
}

func TestSEL_007_monitor_only_label_holds_update(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Labels: map[string]string{"parachute.monitor-only": "true"}})
	id, image := env.ContainerState(a)
	app.publishNewer()
	result := runOnce(env)
	assertState(t, result, a, "held", "monitor_only")
	assertUnchanged(env, a, id, image)
}

func TestSEL_007_global_monitor_only_holds_all(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	b := env.RunContainer("b", app.ref, harness.ContainerOptions{})
	idA, imageA := env.ContainerState(a)
	idB, imageB := env.ContainerState(b)
	app.publishNewer()
	result := runOnce(env, "--monitor-only")
	assertState(t, result, a, "held", "monitor_only")
	assertState(t, result, b, "held", "monitor_only")
	assertUnchanged(env, a, idA, imageA)
	assertUnchanged(env, b, idB, imageB)
}

func TestSEL_008_label_takes_precedence_over_global_monitor_only(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Labels: map[string]string{"parachute.monitor-only": "false"}})
	app.publishNewer()
	result := runOnce(env, "--monitor-only", "--label-take-precedence")
	assertState(t, result, a, "would_update", "")
}

func TestSEL_008_global_monitor_only_wins_without_precedence(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Labels: map[string]string{"parachute.monitor-only": "false"}})
	id, image := env.ContainerState(a)
	app.publishNewer()
	result := runOnce(env, "--monitor-only")
	assertState(t, result, a, "held", "monitor_only")
	assertUnchanged(env, a, id, image)
}

func TestSEL_009_restarting_needs_include_restarting(t *testing.T) {
	env := harness.NewEnv(t)
	image := fixture(env)
	flapping := env.RunContainer("flapping", image, harness.ContainerOptions{Args: []string{"exit", "1"}, Extra: []string{"--restart", "always"}})
	waitRestarting := func() {
		env.Eventually(90*time.Second, "restarting state", func() bool {
			out, _ := env.DockerMayFail("inspect", "--format", "{{.State.Status}} {{.RestartCount}}", flapping)
			fields := strings.Fields(out)
			if len(fields) != 2 {
				return false
			}
			restarts, err := strconv.Atoi(fields[1])
			return err == nil && fields[0] == "restarting" && restarts >= 6
		})
	}
	waitRestarting()
	without := runOnce(env, "--no-pull")
	assertCandidates(t, without, nil, []string{flapping})
	waitRestarting()
	with := runOnce(env, "--no-pull", "--include-restarting")
	assertCandidates(t, with, []string{flapping}, nil)
}

func TestSEL_009_include_stopped_adds_created_and_exited(t *testing.T) {
	env := harness.NewEnv(t)
	image := fixture(env)
	created := env.CreateContainer("created", image, harness.ContainerOptions{})
	exited := env.RunContainer("exited", image, harness.ContainerOptions{Args: []string{"exit", "0"}})
	env.Docker("wait", exited)
	result := runOnce(env, "--include-stopped")
	assertCandidates(t, result, []string{created, exited}, nil)
}

func TestSEL_009_paused_is_never_a_candidate(t *testing.T) {
	env := harness.NewEnv(t)
	image := fixture(env)
	paused := env.RunContainer("paused", image, harness.ContainerOptions{})
	env.Docker("pause", paused)
	result := runOnce(env, "--include-stopped", "--include-restarting")
	assertCandidates(t, result, nil, []string{paused})
}

func TestSEL_010_revive_stopped_requires_include_stopped(t *testing.T) {
	env := harness.NewEnv(t)
	result := runOnce(env, "--revive-stopped")
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, "revive-stopped", "include-stopped")
}

func TestSEL_011_scope_selects_matching_label(t *testing.T) {
	env := harness.NewEnv(t)
	image := fixture(env)
	prod, dev := env.Name("prod"), env.Name("dev")
	a := env.RunContainer("a", image, harness.ContainerOptions{Unscoped: true, Labels: map[string]string{"parachute.scope": prod}})
	b := env.RunContainer("b", image, harness.ContainerOptions{Unscoped: true, Labels: map[string]string{"parachute.scope": dev}})
	c := env.RunContainer("c", image, harness.ContainerOptions{Unscoped: true})
	result := runOnce(env, "--scope", prod)
	assertCandidates(t, result, []string{a}, []string{b, c})
}

func TestSEL_011_scope_none_selects_unlabelled(t *testing.T) {
	env := harness.NewEnv(t)
	image := fixture(env)
	a := env.RunContainer("a", image, harness.ContainerOptions{Unscoped: true, Labels: map[string]string{"parachute.scope": env.Name("prod")}})
	b := env.RunContainer("b", image, harness.ContainerOptions{Unscoped: true, Labels: map[string]string{"parachute.scope": env.Name("dev")}})
	c := env.RunContainer("c", image, harness.ContainerOptions{Unscoped: true})
	result := runOnce(env, "--scope", "none", "--no-pull")
	assertCandidates(t, result, []string{c}, []string{a, b})
}

func TestSEL_012_helper_containers_are_excluded(t *testing.T) {
	env := harness.NewEnv(t)
	image := fixture(env)
	helper := env.RunContainer("helper", image, harness.ContainerOptions{Labels: map[string]string{"parachute.helper": "true"}})
	result := runOnce(env)
	assertCandidates(t, result, nil, []string{helper})
}

func TestSEL_003_own_container_label_does_not_apply(t *testing.T) {
	env := harness.NewEnv(t)
	a := env.RunContainer("a", fixture(env), harness.ContainerOptions{})
	result := env.RunParachuteContainer("parachute", harness.ContainerRun{
		Args:   []string{"--run-once", "--porcelain", "v1", "--no-pull", "--scope", env.Scope()},
		Labels: map[string]string{"parachute.enable": "false"},
	})
	assertExit(t, result, 0)
	assertCandidates(t, result, []string{a}, nil)
}
