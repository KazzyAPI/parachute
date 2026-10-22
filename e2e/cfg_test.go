package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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

func TestCFG_006_unarmed_reports_would_update_and_changes_nothing(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	id, image := env.ContainerState(a)
	images, volumes := env.ImageList(), env.VolumeList()
	result := runOnce(env, "--notification-url", webhook.URL())
	assertExit(t, result, 0)
	assertState(t, result, a, "would_update", "")
	if reports := webhook.Matching(a); len(reports) != 1 || !strings.Contains(reports[0].Message, "would_update") {
		t.Errorf("webhook did not receive a would_update report for %s: %v", a, webhook.Received())
	}
	assertUnchanged(env, a, id, image)
	if env.ImageList() != images || env.VolumeList() != volumes {
		t.Errorf("image or volume list changed")
	}
}

func TestCFG_006_armed_is_refused_in_this_milestone(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once").Setenv("PARACHUTE_ARMED", "true").Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, "updates are not implemented in this milestone")
}

func TestCFG_006_restart_without_armed_changes_nothing(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	id, image := env.ContainerState(a)
	assertExit(t, env.Parachute("--run-once", "--armed").Run(runTimeout), 2)
	result := runOnce(env)
	assertState(t, result, a, "would_update", "")
	assertUnchanged(env, a, id, image)
}

func TestCFG_006_copied_state_file_cannot_arm(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	id, image := env.ContainerState(a)
	images, volumes := env.ImageList(), env.VolumeList()
	copied := `{"format":"parachute.state/v1","armed":true,"last_cycle":null,"results":[]}`
	env.WriteFile("state/state.json", copied, 0o644)
	result := runOnce(env)
	assertState(t, result, a, "would_update", "")
	assertUnchanged(env, a, id, image)
	if env.ImageList() != images || env.VolumeList() != volumes {
		t.Errorf("image or volume list changed")
	}
}

func TestCFG_007_porcelain_lines(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	fresh := env.Registry().Push("fresh", "1", "1")
	updated := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	env.RunContainer("b", fresh, harness.ContainerOptions{})
	env.RunContainer("c", fresh, harness.ContainerOptions{})
	app.publishNewer()
	result := runOnce(env)
	lines := result.StdoutLines()
	if len(lines) != 4 {
		t.Fatalf("expected 4 porcelain lines, got %d:\n%s", len(lines), result.Stdout)
	}
	for _, object := range result.Porcelain() {
		if object["v"] != float64(1) {
			t.Errorf("line without v=1: %v", object)
		}
	}
	assertState(t, result, updated, "would_update", "")
	summary := result.Summary()
	if summary["type"] != "summary" || summary["scanned"] != float64(3) || summary["would_update"] != float64(1) || summary["fresh"] != float64(2) {
		t.Errorf("unexpected summary %v", summary)
	}
	for _, key := range []string{"v", "type", "cycle", "host", "stack", "service", "container", "image", "image_id", "candidate_digest", "state", "reason", "txn", "snapshot"} {
		if _, ok := result.Entry(updated)[key]; !ok {
			t.Errorf("entry misses key %s", key)
		}
	}
}

func TestCFG_007_unknown_porcelain_version(t *testing.T) {
	env := harness.NewEnv(t)
	assertExit(t, env.Parachute("--porcelain", "v2").Run(runTimeout), 2)
}

func TestCFG_008_json_logs(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	result := runOnce(env, "--log-format", "json", "--debug")
	logs := result.Logs()
	if len(logs) == 0 {
		t.Fatal("no log lines")
	}
	for _, entry := range logs {
		if !entry.JSON() || entry.Time().IsZero() || entry.Level() == "" || entry.Msg() == "" {
			t.Errorf("log line is not JSON with time, level and msg: %s", entry.Raw)
		}
	}
	for _, entry := range findLogs(logs, func(entry harness.Log) bool { return entry.Msg() == "container checked" }) {
		for _, key := range []string{"cycle", "host", "stack", "service", "container"} {
			if entry.Get(key) == "" {
				t.Errorf("container line misses %s: %s", key, entry.Raw)
			}
		}
	}
}

func TestCFG_008_no_color_removes_escape_codes(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once", "--log-format", "pretty").Setenv("NO_COLOR", "1").Run(runTimeout)
	assertExit(t, result, 0)
	if strings.Contains(result.Stderr, "\x1b[") {
		t.Errorf("output contains colour codes:\n%q", result.Stderr)
	}
}

func TestCFG_009_failed_candidate_exits_1(t *testing.T) {
	env := harness.NewEnv(t)
	image := env.Tag(fixture(env), "127.0.0.1:1/"+env.ID+"/offline:1")
	env.RunContainer("a", image, harness.ContainerOptions{})
	assertExit(t, runOnce(env), 1)
}

func TestCFG_009_sigterm_while_idle_exits_0(t *testing.T) {
	env := harness.NewEnv(t)
	process := startDaemon(env, env.Parachute())
	process.WaitForLog(30*time.Second, "cycle finished", func(entry harness.Log) bool { return entry.Msg() == "cycle finished" })
	signalled := time.Now()
	process.Signal(syscall.SIGTERM)
	if code := process.Wait(5 * time.Second); code != 0 {
		t.Fatalf("exit code %d, want 0", code)
	}
	if elapsed := time.Since(signalled); elapsed > 5*time.Second {
		t.Fatalf("took %s to exit", elapsed)
	}
}

func TestCFG_010_missing_state_file_is_created(t *testing.T) {
	env := harness.NewEnv(t)
	assertExit(t, env.Parachute("--run-once").Run(runTimeout), 0)
	contents, err := os.ReadFile(filepath.Join(env.StateDir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(contents, &document); err != nil || document["format"] != "parachute.state/v1" {
		t.Fatalf("state.json is not parachute.state/v1: %v %s", err, contents)
	}
}

func TestCFG_010_corrupt_state_file_is_kept(t *testing.T) {
	env := harness.NewEnv(t)
	path := env.WriteFile("state/state.json", "{broken", 0o644)
	result := env.Parachute("--run-once").Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, path)
	if contents, err := os.ReadFile(path); err != nil || string(contents) != "{broken" {
		t.Fatalf("state file changed: %q %v", contents, err)
	}
}

func TestCFG_010_unwritable_state_directory_is_startup_error(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write read-only directories")
	}
	env := harness.NewEnv(t)
	if err := os.Chmod(env.StateDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(env.StateDir, 0o755) })
	result := env.Parachute("--run-once").Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, filepath.Join(env.StateDir, "state.json"))
}

func TestCFG_010_state_file_survives_sigkill(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	env.RunContainer("a", app.ref, harness.ContainerOptions{})
	env.RunContainer("b", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	path := filepath.Join(env.StateDir, "state.json")
	for round := range 20 {
		process := env.Parachute("--interval", "1").Start()
		time.Sleep(time.Duration(300+round*97%900) * time.Millisecond)
		process.Signal(syscall.SIGKILL)
		process.Wait(10 * time.Second)
		contents, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		var document map[string]any
		if err != nil || json.Unmarshal(contents, &document) != nil {
			t.Fatalf("round %d: state.json does not parse: %v %q", round, err, contents)
		}
	}
}

func TestCFG_011_convenient_profile_arms_and_is_refused(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once", "--profile", "convenient").Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, "updates are not implemented in this milestone")
}

func TestCFG_011_explicit_setting_wins_over_profile(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once", "--profile", "convenient", "--armed=false", "--delay-days", "3").Run(runTimeout)
	assertExit(t, result, 0)
	started := findLogs(result.Logs(), func(entry harness.Log) bool { return entry.Msg() == "Parachute started" })
	if len(started) != 1 {
		t.Fatalf("no startup message")
	}
	relaxed := started[0].Get("relaxed")
	if strings.Contains(relaxed, "delay-days") || !strings.Contains(relaxed, "protect-databases") || strings.Contains(relaxed, "armed") {
		t.Errorf("relaxed defaults %q, want only protect-databases", relaxed)
	}
}

func TestCFG_011_startup_message_names_profile_and_relaxations(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	result := env.Parachute("--run-once", "--profile", "convenient", "--armed=false", "--notification-url", webhook.URL()).Run(runTimeout)
	assertExit(t, result, 0)
	startup := webhook.Matching("Parachute started")
	if len(startup) != 1 {
		t.Fatalf("startup messages: %v", webhook.Received())
	}
	assertContains(t, startup[0].Message, "convenient", "delay-days", "protect-databases")
}

func TestCFG_011_unknown_profile(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--profile", "fast").Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, "profile", "fast")
}

func TestCFG_006_armed_label_on_own_container_is_refused(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.RunParachuteContainer("parachute", harness.ContainerRun{
		Args:   []string{"--run-once", "--scope", env.Scope()},
		Labels: map[string]string{"parachute.armed": "true"},
	})
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, "updates are not implemented in this milestone")
}
