package e2e

import (
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func driftOf(t *testing.T, env *harness.Env, declared string) (string, any) {
	t.Helper()
	ref := fixture(env)
	compose := env.WriteFile("shop/compose.yaml", "services:\n  web:\n    image: "+declared+"\n", 0o644)
	labels := composeLabels(env.Name("shop"), "web")
	labels[composeConfigFilesLabel] = compose
	name := env.RunContainer("web", ref, harness.ContainerOptions{Labels: labels})
	checksum := env.Checksum(compose)
	assertExit(t, runOnce(env), 0)
	drift := statusServices(statusDocument(t, env))[name]["drift"]
	if env.Checksum(compose) != checksum {
		t.Error("compose file changed")
	}
	return ref, drift
}

func TestDEP_011_compose_image_differing_from_container_is_drift(t *testing.T) {
	env := harness.NewEnv(t)
	declared := env.Registry().Ref("app", "2")
	ref, drift := driftOf(t, env, declared)
	text, _ := drift.(string)
	assertContains(t, text, declared, ref)
}

func TestDEP_011_variable_image_is_drift_unknown(t *testing.T) {
	env := harness.NewEnv(t)
	_, drift := driftOf(t, env, env.Registry().Ref("app", "${TAG}"))
	if drift != "unknown" {
		t.Errorf("drift %v, want unknown", drift)
	}
}
