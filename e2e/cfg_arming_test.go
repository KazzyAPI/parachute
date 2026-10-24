package e2e

import (
	"strings"
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

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
	if afterImages, afterVolumes := env.ImageList(), env.VolumeList(); afterImages != images || afterVolumes != volumes {
		t.Errorf("image or volume list changed:\nimages before:\n%s\nafter:\n%s\nvolumes before:\n%s\nafter:\n%s", images, afterImages, volumes, afterVolumes)
	}
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
	copied := `{"format":"parachute.state/v1","last_cycle":null,"results":[],"first_seen":[],"approvals":[],"notified":[],"superseded_instances":[]}`
	env.WriteFile("state/state.json", copied, 0o644)
	result := runOnce(env)
	assertState(t, result, a, "would_update", "")
	assertUnchanged(env, a, id, image)
	if afterImages, afterVolumes := env.ImageList(), env.VolumeList(); afterImages != images || afterVolumes != volumes {
		t.Errorf("image or volume list changed:\nimages before:\n%s\nafter:\n%s\nvolumes before:\n%s\nafter:\n%s", images, afterImages, volumes, afterVolumes)
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

func TestCFG_011_explicit_setting_wins_over_profile(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once", "--profile", "convenient", "--armed=false", "--delay-days", "3").Run(runTimeout)
	assertExit(t, result, 0)
	relaxed := startupLog(t, result.Logs()).Get("relaxed")
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
