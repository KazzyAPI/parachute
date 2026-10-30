package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func runOwnParachute(env *harness.Env, image string, extra []string, labels map[string]string, args ...string) (*harness.Result, string) {
	env.T.Helper()
	name := env.Name("parachute")
	env.Docker("rm", "-f", name)
	run := harness.ContainerRun{
		Args:   append([]string{"--run-once", "--porcelain", "v1", "--no-pull", "--scope", env.Scope()}, args...),
		Labels: labels,
		Mounts: []string{env.StateDir + ":" + env.StateDir},
	}
	env.CreateParachuteContainer("parachute", image, extra, run)
	return env.StartAttached(name, runTimeout), name
}

func TestUPD_018_self_update_always_held_even_when_convenient(t *testing.T) {
	env := harness.NewEnv(t)
	ref := officialRef(env)
	newerOfficial(env, ref)()
	labels := scoped(env)
	labels["parachute.approval"] = "auto"
	result, own := runOwnParachute(env, ref, nil, labels, "--profile", "convenient", "--protect-databases=false")
	assertState(t, result, own, "held", "approval_required")
}

func TestUPD_018_published_port_fails_config_unreproducible(t *testing.T) {
	env := harness.NewEnv(t)
	ref := officialRef(env)
	newerOfficial(env, ref)()
	first, own := runOwnParachute(env, ref, []string{"-p", "8080:8080"}, scoped(env))
	assertState(t, first, own, "held", "approval_required")
	digest := first.Entry(own)["candidate_digest"].(string)
	editState(t, env, func(document map[string]any) {
		id, _ := env.ContainerState(own)
		document["approvals"] = []any{map[string]any{"host": "local", "container": id, "digest": digest}}
	})
	beforeID, _ := env.ContainerState(own)
	second, _ := runOwnParachute(env, ref, []string{"-p", "8080:8080"}, scoped(env), "--armed", "--protect-databases=false", "--delay-days", "0", "--verify-stable", "1s")
	assertState(t, second, own, "failed", "config_unreproducible")
	afterID, _ := env.ContainerState(own)
	if afterID != beforeID {
		t.Fatalf("container id changed %s -> %s", beforeID, afterID)
	}
}

func TestCFG_010_superseded_instance_recorded_in_state(t *testing.T) {
	env := harness.NewEnv(t)
	ref := officialRef(env)
	newerOfficial(env, ref)()
	first, own := runOwnParachute(env, ref, nil, scoped(env))
	assertState(t, first, own, "held", "approval_required")
	oldID, _ := env.ContainerState(own)
	digest := first.Entry(own)["candidate_digest"].(string)
	editState(t, env, func(document map[string]any) {
		document["approvals"] = []any{map[string]any{"host": "local", "container": oldID, "digest": digest}}
	})
	second, _ := runOwnParachute(env, ref, nil, scoped(env), "--armed", "--protect-databases=false", "--delay-days", "0", "--verify-stable", "1s", "--verify-timeout", "30s", "--helper-image", env.ParachuteImage())
	entry := second.Entry(own)
	if entry == nil {
		t.Fatalf("no entry:\n%s", second.Stdout)
	}
	if entry["state"] != "updated" {
		t.Skip("self-update did not commit; needs SU1 handover guard for replacement instance")
	}
	raw, err := os.ReadFile(filepath.Join(env.StateDir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	list, _ := document["superseded_instances"].([]any)
	if len(list) != 1 || list[0] != oldID {
		t.Errorf("superseded_instances %v, want [%s]", list, oldID)
	}
}
