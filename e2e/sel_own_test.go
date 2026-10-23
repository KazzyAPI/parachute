package e2e

import (
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestSEL_011_own_container_outside_scope_is_not_a_candidate(t *testing.T) {
	env := harness.NewEnv(t)
	ref := officialRef(env)
	result, own := runOwnContainer(env, ref, nil, nil, newerOfficial(env, ref))
	assertNoEntry(t, result, own)
}

func TestSEL_012_creation_helper_label_excludes(t *testing.T) {
	env := harness.NewEnv(t)
	helper := env.RunContainer("helper", fixture(env), harness.ContainerOptions{Labels: map[string]string{"parachute.helper": "true"}})
	assertNoEntry(t, runOnce(env), helper)
}

func TestSEL_012_image_helper_label_does_not_exclude(t *testing.T) {
	env := harness.NewEnv(t)
	app, publishNewer := labelledUpdatable(env, map[string]string{"parachute.helper": "true"})
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	publishNewer()
	assertHasEntry(t, runOnce(env), a)
}

func TestSEL_012_other_instance_with_same_scope_is_excluded(t *testing.T) {
	env := harness.NewEnv(t)
	ref := officialRef(env)
	other := env.Name("other")
	env.Docker("run", "-d", "--name", other, "--label", "parachute-e2e="+env.ID, "--label", "parachute.scope="+env.Scope(), "--entrypoint", "sleep", ref, "infinity")
	result, own := runOwnContainer(env, ref, nil, scoped(env), newerOfficial(env, ref))
	assertHasEntry(t, result, own)
	assertNoEntry(t, result, other)
}

func TestSEL_012_own_container_found_without_labels(t *testing.T) {
	env := harness.NewEnv(t)
	ref := env.Name("parachute") + ":1"
	env.BuildParachuteImage(ref, "1", nil)
	result, own := runOwnContainer(env, ref, nil, scoped(env), func() { env.BuildParachuteImage(ref, "2", nil) })
	assertState(t, result, own, "held", "approval_required")
}

func TestSEL_012_own_container_found_with_custom_hostname(t *testing.T) {
	env := harness.NewEnv(t)
	ref := officialRef(env)
	result, own := runOwnContainer(env, ref, []string{"--hostname", "parachute-main"}, scoped(env), newerOfficial(env, ref))
	assertState(t, result, own, "held", "approval_required")
}

func TestSEL_012_self_label_on_other_container_excludes(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Labels: map[string]string{selfLabel: "true"}})
	id, _ := env.ContainerState(a)
	app.publishNewer()
	assertNoEntry(t, runOnce(env), a)
	if gotID, _ := env.ContainerState(a); gotID != id {
		t.Errorf("%s changed: id %s -> %s", a, id, gotID)
	}
}
