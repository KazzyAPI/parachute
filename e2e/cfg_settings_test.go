package e2e

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestCFG_001_environment_variable_sets_interval(t *testing.T) {
	env := harness.NewEnv(t)
	process := startDaemon(env, env.Parachute().Setenv("PARACHUTE_INTERVAL", "60"))
	started := cycleStart(t, firstCycle(t, process).Get("cycle"))
	assertDuration(t, announcedNext(t, process, 1).Sub(started), 60*time.Second, 5*time.Second)
}

func TestCFG_001_environment_variable_sets_stop_timeout(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	spec := func(version string) harness.ImageSpec {
		return harness.ImageSpec{Base: "alpine:3.22", Version: version, Cmd: []string{"sleep", "infinity"}}
	}
	a := env.RunContainer("a", registry.PushSpec("stubborn", "1", spec("1")), harness.ContainerOptions{})
	id, _ := env.ContainerState(a)
	registry.PushSpec("stubborn", "1", spec("2"))
	since := time.Now().Add(-time.Second).Unix()
	result := armedParachute(env, "--backup", "none").Setenv("PARACHUTE_STOP_TIMEOUT", "5s").Run(runTimeout)
	assertState(t, result, a, "updated", "")
	events := env.Docker("events", "--since", strconv.FormatInt(since, 10), "--until", strconv.FormatInt(time.Now().Unix()+1, 10),
		"--filter", "container="+id, "--filter", "event=kill", "--format", "{{.Actor.Attributes.signal}} {{.TimeNano}}")
	signalled := map[string]time.Time{}
	for _, line := range strings.Split(strings.TrimSpace(events), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		nanos, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			t.Fatalf("event line %q: %v", line, err)
		}
		signalled[fields[0]] = time.Unix(0, nanos)
	}
	term, kill := signalled["15"], signalled["9"]
	if term.IsZero() || kill.IsZero() {
		t.Fatalf("kill events missing SIGTERM or SIGKILL:\n%s", events)
	}
	assertDuration(t, kill.Sub(term), 5*time.Second, 2*time.Second)
}

func TestCFG_002_flag_wins_over_environment(t *testing.T) {
	env := harness.NewEnv(t)
	process := startDaemon(env, env.Parachute("--interval", "120").Setenv("PARACHUTE_INTERVAL", "60"))
	started := cycleStart(t, firstCycle(t, process).Get("cycle"))
	assertDuration(t, announcedNext(t, process, 1).Sub(started), 120*time.Second, 5*time.Second)
}

func TestCFG_003_quoted_mixed_separator_list(t *testing.T) {
	env := harness.NewEnv(t)
	image := fixture(env)
	a := env.RunContainer("a", image, harness.ContainerOptions{})
	b := env.RunContainer("b", image, harness.ContainerOptions{})
	c := env.RunContainer("c", image, harness.ContainerOptions{})
	d := env.RunContainer("d", image, harness.ContainerOptions{})
	result := env.Parachute("--run-once", "--porcelain", "v1").Setenv("PARACHUTE_DISABLE_CONTAINERS", `"`+a+", "+b+" "+c+`"`).Run(runTimeout)
	assertCandidates(t, result, []string{d}, []string{a, b, c})
}

func TestCFG_003_invalid_boolean_is_startup_error(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once").Setenv("PARACHUTE_CLEANUP", "yes").Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, "cleanup", "yes")
}

func TestCFG_004_secret_value_read_from_file(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	secret := env.WriteFile("secrets/notification-url", webhook.URL()+"\n", 0o600)
	result := env.Parachute("--run-once").Setenv("PARACHUTE_NOTIFICATION_URL", secret).Run(runTimeout)
	assertExit(t, result, 0)
	if len(webhook.Matching("Parachute started")) != 1 {
		t.Fatalf("webhook from file URL received %v", webhook.Received())
	}
}

func TestCFG_004_unreadable_secret_file_is_startup_error(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read files with mode 000")
	}
	env := harness.NewEnv(t)
	secret := env.WriteFile("secrets/notification-url", "generic://127.0.0.1:1/x\n", 0o000)
	result := env.Parachute("--run-once").Setenv("PARACHUTE_NOTIFICATION_URL", secret).Run(runTimeout)
	assertExit(t, result, 2)
}

func TestCFG_005_unknown_flag_suggests_closest(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--intervall", "60").Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, "--intervall", "--interval")
}

func TestCFG_005_unknown_environment_variable(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once").Setenv("PARACHUTE_FOO", "1").Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, "PARACHUTE_FOO")
}

func TestCFG_005_unknown_label_is_warned_once(t *testing.T) {
	env := harness.NewEnv(t)
	image := fixture(env)
	a := env.RunContainer("a", image, harness.ContainerOptions{Labels: map[string]string{"parachute.enabel": "false"}})
	result := runOnce(env)
	warnings := findLogs(result.Logs(), func(entry harness.Log) bool {
		return entry.Level() == "warn" && entry.Get("container") == a && entry.Get("label") == "parachute.enabel"
	})
	if len(warnings) != 1 {
		t.Errorf("expected one warning naming %s and parachute.enabel, got %d", a, len(warnings))
	}
	assertCandidates(t, result, []string{a}, nil)
}
