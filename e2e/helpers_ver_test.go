package e2e

import (
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func armedRunOnce(env *harness.Env, args ...string) *harness.Result {
	env.T.Helper()
	flags := []string{"--armed", "--delay-days", "0", "--protect-databases=false", "--backup", "none", "--helper-image", env.ParachuteImage()}
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
