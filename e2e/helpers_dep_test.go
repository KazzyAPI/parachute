package e2e

import (
	"github.com/KazzyAPI/parachute/e2e/harness"
)

func armedOnce(env *harness.Env, args ...string) *harness.Result {
	env.T.Helper()
	return runOnce(env, append([]string{"--armed", "--protect-databases=false"}, args...)...)
}

func composeUp(env *harness.Env, project, compose string) {
	env.T.Helper()
	if _, err := env.DockerMayFail("compose", "version"); err != nil {
		env.T.Skip("docker compose is not installed")
	}
	env.Docker("compose", "-p", project, "-f", compose, "up", "-d")
}
