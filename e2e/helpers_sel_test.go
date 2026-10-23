package e2e

import (
	"strings"
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

const selfLabel = "parachute.self"

func officialRef(env *harness.Env) string {
	env.T.Helper()
	return env.Tag(env.ParachuteImage(), env.Name("parachute")+":1")
}

func newerOfficial(env *harness.Env, ref string) func() {
	return func() { env.BuildParachuteImage(ref, "2", map[string]string{selfLabel: "true"}) }
}

func runOwnContainer(env *harness.Env, image string, extra []string, labels map[string]string, publishNewer func()) (*harness.Result, string) {
	env.T.Helper()
	args := []string{"--run-once", "--porcelain", "v1", "--no-pull", "--scope", env.Scope()}
	name := env.CreateParachuteContainer("parachute", image, extra, harness.ContainerRun{Args: args, Labels: labels})
	publishNewer()
	result := env.StartAttached(name, runTimeout)
	assertExit(env.T, result, 0)
	return result, name
}

func scoped(env *harness.Env) map[string]string {
	return map[string]string{"parachute.scope": env.Scope()}
}

func labelledUpdatable(env *harness.Env, labels map[string]string) (updatable, func()) {
	env.T.Helper()
	registry := env.Registry()
	ref := registry.PushSpec("app", "1", harness.ImageSpec{Version: "1", Labels: labels})
	return updatable{registry: registry, ref: ref}, func() { registry.PushSpec("app", "1", harness.ImageSpec{Version: "2", Labels: labels}) }
}

func labelWarnings(result *harness.Result, container, label string) []harness.Log {
	return findLogs(result.Logs(), func(entry harness.Log) bool {
		return entry.Level() == "warn" && entry.Get("container") == container && strings.Contains(entry.Raw, label)
	})
}

func assertNoEntry(t *testing.T, result *harness.Result, container string) {
	t.Helper()
	if entry := result.Entry(container); entry != nil {
		t.Errorf("unexpected report entry for %s: %v", container, entry)
	}
}

func assertEntry(t *testing.T, result *harness.Result, container string) {
	t.Helper()
	if result.Entry(container) == nil {
		t.Errorf("no report entry for %s:\n%s", container, result.Stdout)
	}
}
