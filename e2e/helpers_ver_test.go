package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func skipUnlessArmedUpdates(t *testing.T, env *harness.Env) {
	t.Helper()
	result := env.Parachute("--run-once", "--armed").Run(30 * time.Second)
	if result.Code == 2 && strings.Contains(result.Stderr, "not implemented") {
		t.Skip("armed updates are not available in this build")
	}
}

func armedRunOnce(env *harness.Env, args ...string) *harness.Result {
	env.T.Helper()
	flags := []string{"--armed", "--delay-days", "0", "--protect-databases=false", "--backup", "none"}
	return runOnce(env, append(flags, args...)...)
}

func assertRolledBack(t *testing.T, result *harness.Result, container, reason string) {
	t.Helper()
	assertState(t, result, container, "rolled_back", reason)
}

func assertUpdated(t *testing.T, result *harness.Result, container string) {
	t.Helper()
	assertState(t, result, container, "updated", "")
}
