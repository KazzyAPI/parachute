package e2e

import (
	"strings"
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestCFG_013_help_lists_commands_and_arming(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--help").Run(runTimeout)
	assertExit(t, result, 0)
	assertContains(t, result.Stdout, "Usage:", "status", "healthcheck", "--armed")
}

func TestCFG_013_version_prints_the_version(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("version").Run(runTimeout)
	assertExit(t, result, 0)
	if !strings.HasPrefix(result.Stdout, "parachute ") {
		t.Fatalf("stdout %q", result.Stdout)
	}
}

func TestCFG_013_unwritable_state_directory_names_the_volume_to_mount(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once", "--state-dir", "/nonexistent/parachute").Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, "/nonexistent/parachute", "-v parachute-state:")
}

func TestCFG_013_unarmed_startup_message_says_how_to_arm(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	assertExit(t, env.Parachute("--run-once", "--notification-url", webhook.URL()).Run(runTimeout), 0)
	startup := webhook.Matching("Parachute started")
	if len(startup) != 1 {
		t.Fatalf("expected one startup message, got %v", webhook.Received())
	}
	assertContains(t, startup[0].Message, "PARACHUTE_ARMED=true")
}

func TestCFG_013_armed_startup_message_has_no_arming_hint(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	result := armedParachute(env, "--notification-url", webhook.URL()).Run(runTimeout)
	assertExit(t, result, 0)
	for _, startup := range webhook.Matching("Parachute started") {
		if strings.Contains(startup.Message, "PARACHUTE_ARMED=true") {
			t.Fatalf("armed startup message carries the hint: %s", startup.Message)
		}
	}
}
