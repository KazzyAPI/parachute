package e2e

import (
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

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
