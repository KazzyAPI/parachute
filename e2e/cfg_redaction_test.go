package e2e

import (
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestCFG_012_logs_and_porcelain_carry_no_secrets(t *testing.T) {
	env := harness.NewEnv(t)
	planted := plantSecrets(env)
	result := runOnce(env, append(planted.args, "--log-level", "debug")...)
	assertState(t, result, planted.container, "would_update", "")
	assertNoSecrets(t, "stdout", result.Stdout, planted.secrets...)
	assertNoSecrets(t, "stderr", result.Stderr, planted.secrets...)
}

func TestCFG_012_invalid_secret_value_is_not_shown(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once", "--notification-url", "generic://bob:hunter2@exa mple").Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, "notification-url")
	assertNoSecrets(t, "stderr", result.Stderr, "hunter2")
}
