package e2e

import (
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestHOK_001_enable_lifecycle_hooks_is_not_startup_error(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once", "--enable-lifecycle-hooks").Run(runTimeout)
	assertExit(t, result, 0)
}

func TestHOK_001_environment_variable_enables_hooks(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once").Setenv("PARACHUTE_ENABLE_LIFECYCLE_HOOKS", "true").Run(runTimeout)
	assertExit(t, result, 0)
}
