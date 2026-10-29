package e2e

import (
	"strings"
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func armedParachute(env *harness.Env, args ...string) *harness.Parachute {
	env.T.Helper()
	base := []string{"--run-once", "--armed", "--protect-databases=false", "--porcelain", "v1", "--helper-image", env.ParachuteImage(), "--verify-stable", "1s", "--verify-timeout", "30s"}
	return env.Parachute(append(base, args...)...)
}

func rollbackCandidate(env *harness.Env, repo string) string {
	env.T.Helper()
	labels := map[string]string{"parachute.verify.stable": "2s"}
	registry := env.Registry()
	ref := registry.PushSpec(repo, "1", harness.ImageSpec{Verify: true, Version: "1"})
	container := env.RunContainer(repo, ref, harness.ContainerOptions{Labels: labels})
	registry.PushSpec(repo, "1", harness.ImageSpec{Verify: true, Version: "2", Env: map[string]string{"EXIT_AFTER": "2"}})
	return container
}

func rollbackFailedRun(env *harness.Env, repo string, withRolledBack bool) (result *harness.Result, rolledBack, rollbackFailed string) {
	env.T.Helper()
	rollbackFailed = rollbackCandidate(env, repo)
	if withRolledBack {
		rolledBack = rollbackCandidate(env, repo+"-ok")
	}
	id, _ := env.ContainerState(rollbackFailed)
	proxy := env.HTTPEngineProxy()
	proxy.Fail("/containers/" + id + "/start")
	result = armedParachute(env, "--backup", "none", "--allow-insecure-tcp").Setenv("DOCKER_HOST", proxy.Endpoint).Run(runTimeout)
	return result, rolledBack, rollbackFailed
}

func requestsMatching(requests []string, parts ...string) []string {
	var matching []string
	for _, request := range requests {
		if all(request, parts) {
			matching = append(matching, request)
		}
	}
	return matching
}

func all(text string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}

func assertRollbackFailed(t *testing.T, result *harness.Result, container string) {
	t.Helper()
	assertState(t, result, container, "rollback_failed", "restore_failed")
}
