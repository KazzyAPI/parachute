package e2e

import (
	"strings"
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func createOwnParachute(env *harness.Env, image string, extra []string, labels map[string]string, args ...string) string {
	env.T.Helper()
	run := harness.ContainerRun{
		Args:   append([]string{"--run-once", "--porcelain", "v1", "--no-pull", "--scope", env.Scope(), "--state-dir", "/var/lib/parachute"}, args...),
		Labels: labels,
		Mounts: []string{env.StateDir + ":/var/lib/parachute"},
	}
	return env.CreateParachuteContainer("parachute", image, extra, run)
}

func TestUPD_018_self_update_always_held_even_when_convenient(t *testing.T) {
	env := harness.NewEnv(t)
	ref := officialRef(env)
	labels := scoped(env)
	labels["parachute.approval"] = "auto"
	own := createOwnParachute(env, ref, nil, labels, "--profile", "convenient", "--protect-databases=false")
	newerOfficial(env, ref)()
	result := env.StartAttached(own, runTimeout)
	assertExit(t, result, 0)
	assertState(t, result, own, "held", "approval_required")
}

func TestUPD_018_published_port_fails_config_unreproducible(t *testing.T) {
	env := harness.NewEnv(t)
	ref := officialRef(env)
	own := createOwnParachute(env, ref, []string{"-p", "8080"}, scoped(env), "--armed", "--protect-databases=false", "--delay-days", "0", "--verify-stable", "1s")
	newerOfficial(env, ref)()
	first := env.StartAttached(own, runTimeout)
	assertExit(t, first, 0)
	digest := assertState(t, first, own, "held", "approval_required")["candidate_digest"].(string)
	env.OpenStateDir()
	beforeID, _ := env.ContainerState(own)
	editState(t, env, func(document map[string]any) {
		document["approvals"] = []any{map[string]any{"host": "local", "container": beforeID, "digest": digest}}
	})
	second := env.StartAttached(own, runTimeout)
	assertExit(t, second, 1)
	assertState(t, second, own, "failed", "config_unreproducible")
	if !strings.Contains(second.Stderr, "published port 8080") {
		t.Errorf("stderr does not name the published port:\n%s", second.Stderr)
	}
	afterID, _ := env.ContainerState(own)
	if afterID != beforeID {
		t.Fatalf("container id changed %s -> %s", beforeID, afterID)
	}
}
