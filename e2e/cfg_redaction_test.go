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

func TestCFG_012_word_like_query_secret_does_not_unmask_url_userinfo(t *testing.T) {
	env := harness.NewEnv(t)
	host := challengeRegistry(env, func(string) string { return "https://realmusr9@127.0.0.1:1/token" }, 0, "")
	a := challengedContainer(env, host)
	result := runOnce(env, "--log-level", "debug", "--notification-url", "generic://127.0.0.1:1/hook?scheme=https&proto=https")
	assertState(t, result, a, "failed", "registry_unreachable")
	assertContains(t, result.Stderr, "[REDACTED]@127.0.0.1:1")
	assertNoSecrets(t, "stderr", result.Stderr, "realmusr9")
	assertNoSecrets(t, "stdout", result.Stdout, "realmusr9")
}
