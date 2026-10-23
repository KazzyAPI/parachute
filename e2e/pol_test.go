package e2e

import (
	"syscall"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestPOL_001_database_inside_delay_outside_window_is_approval_required(t *testing.T) {
	env := harness.NewEnv(t)
	db := newUpdatable(env, "postgres")
	name := env.RunContainer("db", db.ref, harness.ContainerOptions{})
	db.publishNewer()
	result := runOnce(env, "--delay-days", "7", "--maintenance-window", cronAt(time.Now().Add(12*time.Hour)), "--maintenance-window-duration", "1h")
	assertState(t, result, name, "held", "approval_required")
}

func TestPOL_001_delay_hold_reports_its_end_time(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	result := notify(env, webhook, "--delay-days", "1")
	assertState(t, result, a, "held", "delay")
	message := singleReport(t, webhook).Message
	assertContains(t, message, "delay")
	reportedHoldEnd(t, message, "delay")
}

func TestPOL_002_update_waits_for_the_delay(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	process := startDaemon(env, env.Parachute("--porcelain", "v1", "--interval", "30", "--delay-days", "0.0005"))
	waitForEntry(env, process, a, func(entry harness.Log) bool { return entry.Get("state") == "would_update" })
	entries := entriesFor(process, a)
	assertEntry(t, entries[0], "held", "delay")
	if len(entries) != 3 {
		t.Errorf("expected the cycle at 60 seconds to be the first would_update, got %d entries", len(entries))
	}
}

func TestPOL_002_new_digest_restarts_the_delay(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	process := startDaemon(env, env.Parachute("--porcelain", "v1", "--interval", "30", "--delay-days", "0.0005"))
	first := waitForEntry(env, process, a, func(harness.Log) bool { return true })
	waitForEntry(env, process, a, func(entry harness.Log) bool { return entry.Get("cycle") != first.Get("cycle") })
	app.registry.Push(refRepo(app.ref), "1", "3")
	third := waitForEntry(env, process, a, func(entry harness.Log) bool { return entry.Get("candidate_digest") != first.Get("candidate_digest") })
	assertEntry(t, third, "held", "delay")
}

func TestPOL_002_label_zero_updates_in_the_same_cycle(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Labels: map[string]string{"parachute.delay-days": "0"}})
	app.publishNewer()
	assertState(t, runOnce(env, "--delay-days", "2"), a, "would_update", "")
}

func TestPOL_002_image_created_time_is_not_used(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.registry.PushEpoch(refRepo(app.ref), "1", "2")
	result := notify(env, webhook, "--delay-days", "0.0005")
	entry := assertState(t, result, a, "held", "delay")
	end := reportedHoldEnd(t, singleReport(t, webhook).Message, "delay")
	assertDuration(t, end.Sub(cycleStart(t, entry["cycle"].(string))), 43200*time.Millisecond, 3*time.Second)
}

func TestPOL_003_recognised_database_is_held(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	ref := registry.Push("library/postgres", "16", "1")
	db := env.RunContainer("db", ref, harness.ContainerOptions{})
	id, image := env.ContainerState(db)
	registry.Push("library/postgres", "16", "2")
	assertState(t, runOnce(env), db, "held", "approval_required")
	assertUnchanged(env, db, id, image)
}

func TestPOL_003_database_label_false_overrides_recognition(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	ref := registry.Push("library/postgres", "16", "1")
	db := env.RunContainer("db", ref, harness.ContainerOptions{Labels: map[string]string{"parachute.database": "false"}})
	registry.Push("library/postgres", "16", "2")
	assertState(t, runOnce(env), db, "would_update", "")
}

func TestPOL_003_database_label_true_protects_any_image(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	ref := registry.Push("fixture/app", "1", "1")
	a := env.RunContainer("a", ref, harness.ContainerOptions{Labels: map[string]string{"parachute.database": "true"}})
	registry.Push("fixture/app", "1", "2")
	assertState(t, runOnce(env), a, "held", "approval_required")
}

func TestPOL_006_approved_database_update_proceeds(t *testing.T) {
	env := harness.NewEnv(t)
	db := newUpdatable(env, "postgres")
	name := env.RunContainer("db", db.ref, harness.ContainerOptions{})
	db.publishNewer()
	daemon := startApprovalDaemon(env, 5)
	digest := daemon.approveHeld(name)
	entry := waitForEntry(env, daemon.process, name, func(entry harness.Log) bool { return entry.Get("state") != "held" })
	assertEntry(t, entry, "would_update", "")
	if entry.Get("candidate_digest") != digest {
		t.Errorf("approved %s, next cycle had %s", digest, entry.Get("candidate_digest"))
	}
}

func TestPOL_006_approval_lapses_on_a_new_digest(t *testing.T) {
	env := harness.NewEnv(t)
	db := newUpdatable(env, "postgres")
	name := env.RunContainer("db", db.ref, harness.ContainerOptions{})
	db.publishNewer()
	daemon := startApprovalDaemon(env, 15)
	digest := daemon.approveHeld(name)
	db.registry.Push(refRepo(db.ref), "1", "3")
	entry := waitForEntry(env, daemon.process, name, func(entry harness.Log) bool { return entry.Get("candidate_digest") != digest })
	assertEntry(t, entry, "held", "approval_required")
}

func TestPOL_007_closed_window_holds_until_next_start(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	start := time.Now().Add(12 * time.Hour).Truncate(time.Second)
	result := notify(env, webhook, "--maintenance-window", cronAt(start), "--maintenance-window-duration", "1h")
	assertState(t, result, a, "held", "maintenance_window")
	if end := reportedHoldEnd(t, singleReport(t, webhook).Message, "maintenance_window"); !end.Equal(start) {
		t.Errorf("next window start %s, want %s", end, start)
	}
}

func TestPOL_007_open_window_applies_the_update(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	result := runOnce(env, "--maintenance-window", cronAt(time.Now().Add(-10*time.Minute)), "--maintenance-window-duration", "1h")
	assertState(t, result, a, "would_update", "")
}

func TestPOL_008_label_wins_over_global_delay(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Labels: map[string]string{"parachute.delay-days": "0"}})
	app.publishNewer()
	assertState(t, runOnce(env, "--delay-days", "7"), a, "would_update", "")
}

func TestPOL_008_invalid_label_fails_only_its_container(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Labels: map[string]string{"parachute.delay-days": "soon"}})
	b := env.RunContainer("b", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	result := runOnce(env)
	assertState(t, result, a, "failed", "invalid_label")
	assertState(t, result, b, "would_update", "")
	details := findLogs(result.Logs(), func(entry harness.Log) bool {
		return entry.Msg() == "container checked" && entry.Get("container") == a
	})
	if len(details) != 1 {
		t.Fatalf("expected one result log line for %s, got %d", a, len(details))
	}
	assertContains(t, details[0].Get("detail"), "parachute.delay-days")
}

func TestPOL_009_global_relaxation_is_logged_and_announced(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	result := env.Parachute("--run-once", "--notification-url", webhook.URL()).Setenv("PARACHUTE_BACKUP", "none").Run(runTimeout)
	assertExit(t, result, 0)
	warnings := findLogs(result.Logs(), func(entry harness.Log) bool {
		return entry.Level() == "warn" && entry.Get("setting") == "backup" && entry.Get("value") == "none"
	})
	if len(warnings) != 1 {
		t.Errorf("expected one warn line naming backup and none, got %d", len(warnings))
	}
	started := webhook.Matching("Parachute started")
	if len(started) != 1 {
		t.Fatalf("expected one startup message, got %d", len(started))
	}
	assertContains(t, started[0].Message, "relaxed safety defaults", "backup")
}

func TestPOL_009_label_relaxation_is_logged_once(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Labels: map[string]string{"parachute.approval": "auto"}})
	process := startDaemon(env, env.Parachute("--porcelain", "v1", "--interval", "3"))
	process.WaitForLogs(30*time.Second, "two cycles", 2, func(entry harness.Log) bool { return entry.Msg() == "cycle finished" })
	warnings := findLogs(process.Logs(), func(entry harness.Log) bool {
		return entry.Level() == "warn" && entry.Get("container") == a && entry.Get("label") == "parachute.approval"
	})
	if len(warnings) != 1 {
		t.Errorf("expected one relaxation warn line for %s, got %d", a, len(warnings))
	}
}

func TestPOL_011_approval_is_bound_to_the_container_id(t *testing.T) {
	env := harness.NewEnv(t)
	db := newUpdatable(env, "postgres")
	name := env.RunContainer("db", db.ref, harness.ContainerOptions{})
	_, original := env.ContainerState(name)
	db.publishNewer()
	daemon := startApprovalDaemon(env, 300)
	daemon.approveHeld(name)
	daemon.process.Signal(syscall.SIGTERM)
	daemon.process.Wait(30 * time.Second)
	env.Docker("rm", "-f", name)
	env.Tag(original, db.ref)
	env.RunContainer("db", db.ref, harness.ContainerOptions{})
	id, image := env.ContainerState(name)
	assertState(t, runOnce(env), name, "held", "approval_required")
	assertUnchanged(env, name, id, image)
}
