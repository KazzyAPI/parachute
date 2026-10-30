package e2e

import (
	"strings"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func composeUp(env *harness.Env, project, compose string) {
	env.T.Helper()
	if _, err := env.DockerMayFail("compose", "version"); err != nil {
		env.T.Skip("docker compose is not installed")
	}
	env.Docker("compose", "-p", project, "-f", compose, "up", "-d")
}

func containerStartedAt(env *harness.Env, name string) string {
	env.T.Helper()
	return strings.TrimSpace(env.Docker("inspect", "--format", "{{.State.StartedAt}}", name))
}

func containerRestartCount(env *harness.Env, name string) string {
	env.T.Helper()
	return strings.TrimSpace(env.Docker("inspect", "--format", "{{.RestartCount}}", name))
}

func assertTimingUnchanged(env *harness.Env, name, startedAt, restartCount string) {
	env.T.Helper()
	if got := containerStartedAt(env, name); got != startedAt {
		env.T.Errorf("%s StartedAt changed %s -> %s", name, startedAt, got)
	}
	if got := containerRestartCount(env, name); got != restartCount {
		env.T.Errorf("%s RestartCount changed %s -> %s", name, restartCount, got)
	}
}

func waitEventOrder(env *harness.Env, rec *harness.EventRecorder, timeout time.Duration, want ...string) {
	env.T.Helper()
	rec.Wait(timeout, want...)
}
