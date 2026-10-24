package e2e

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

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
	result := armedParachute(env, "--monitor-only", "--label-take-precedence").Run(runTimeout)
	assertState(t, result, a, "updated", "")
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
		env.Eventually(150*time.Second, "restarting state", func() bool {
			out, _ := env.DockerMayFail("inspect", "--format", "{{.State.Status}} {{.RestartCount}}", flapping)
			fields := strings.Fields(out)
			if len(fields) != 2 {
				return false
			}
			restarts, err := strconv.Atoi(fields[1])
			return err == nil && fields[0] == "restarting" && restarts >= 8
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

func stoppedUpdatable(env *harness.Env) string {
	env.T.Helper()
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	env.Docker("stop", a)
	app.publishNewer()
	return a
}

func containerStatus(env *harness.Env, container string) string {
	env.T.Helper()
	return strings.TrimSpace(env.Docker("inspect", "--format", "{{.State.Status}}", container))
}

func TestSEL_010_stopped_container_stays_stopped_after_update(t *testing.T) {
	env := harness.NewEnv(t)
	a := stoppedUpdatable(env)
	_, image := env.ContainerState(a)
	result := armedParachute(env, "--include-stopped").Run(runTimeout)
	assertState(t, result, a, "updated", "")
	if _, after := env.ContainerState(a); after == image {
		t.Error("container still uses the old image")
	}
	if status := containerStatus(env, a); status != "created" && status != "exited" {
		t.Errorf("replacement is %s, want created or exited", status)
	}
}

func TestSEL_010_revive_stopped_starts_replacement(t *testing.T) {
	env := harness.NewEnv(t)
	a := stoppedUpdatable(env)
	result := armedParachute(env, "--include-stopped", "--revive-stopped").Run(runTimeout)
	assertState(t, result, a, "updated", "")
	if status := containerStatus(env, a); status != "running" {
		t.Errorf("replacement is %s, want running", status)
	}
}

func TestSEL_011_two_instances_update_each_container_once(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	prod, dev := env.Name("prod"), env.Name("dev")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Unscoped: true, Labels: map[string]string{"parachute.scope": prod}})
	b := env.RunContainer("b", app.ref, harness.ContainerOptions{Unscoped: true, Labels: map[string]string{"parachute.scope": dev}})
	app.publishNewer()
	idB, imageB := env.ContainerState(b)
	first := armedParachute(env, "--scope", prod).Run(runTimeout)
	assertState(t, first, a, "updated", "")
	assertCandidates(t, first, []string{a}, []string{b})
	assertUnchanged(env, b, idB, imageB)
	idA, imageA := env.ContainerState(a)
	second := armedParachute(env, "--scope", dev).Run(runTimeout)
	assertState(t, second, b, "updated", "")
	assertCandidates(t, second, []string{b}, []string{a})
	assertUnchanged(env, a, idA, imageA)
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
