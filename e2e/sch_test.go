package e2e

import (
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func cycleStarts(t *testing.T, logs []harness.Log) []time.Time {
	t.Helper()
	var starts []time.Time
	for _, entry := range findLogs(logs, func(entry harness.Log) bool { return entry.Msg() == "cycle started" }) {
		starts = append(starts, entry.Time())
	}
	return starts
}

func TestSCH_001_default_interval_is_one_day(t *testing.T) {
	env := harness.NewEnv(t)
	launched := time.Now()
	process := startDaemon(env, env.Parachute())
	first := firstCycle(t, process)
	if delay := first.Received.Sub(launched); delay > 10*time.Second {
		t.Errorf("first cycle started after %s", delay)
	}
	assertDuration(t, announcedNext(t, process, 1).Sub(cycleStart(t, first.Get("cycle"))), 86400*time.Second, 5*time.Second)
}

func TestSCH_001_cycles_repeat_at_the_interval(t *testing.T) {
	env := harness.NewEnv(t)
	process := startDaemon(env, env.Parachute("--interval", "60"))
	process.WaitForLogs(180*time.Second, "three cycles", 3, func(entry harness.Log) bool { return entry.Msg() == "cycle started" })
	starts := cycleStarts(t, process.Logs())[:3]
	for index := 1; index < len(starts); index++ {
		assertDuration(t, starts[index].Sub(starts[index-1]), 60*time.Second, 5*time.Second)
	}
}

func TestSCH_001_invalid_interval(t *testing.T) {
	env := harness.NewEnv(t)
	for _, value := range []string{"0", "1h"} {
		result := env.Parachute("--interval", value).Run(runTimeout)
		assertExit(t, result, 2)
		assertContains(t, result.Stderr, "interval")
	}
}

func TestSCH_002_cron_schedule_fires_on_matching_seconds(t *testing.T) {
	env := harness.NewEnv(t)
	launched := time.Now()
	process := startDaemon(env, env.Parachute("--schedule", "*/20 * * * * *"))
	process.WaitForLogs(120*time.Second, "three cycles", 3, func(entry harness.Log) bool { return entry.Msg() == "cycle started" })
	starts := cycleStarts(t, process.Logs())
	for _, start := range starts {
		if start.Second()%20 != 0 {
			t.Errorf("cycle started at second %d", start.Second())
		}
	}
	if starts[0].Sub(launched) < 500*time.Millisecond && launched.Second()%20 != 0 {
		t.Errorf("a cycle ran at startup")
	}
}

func TestSCH_002_cron_announces_next_run(t *testing.T) {
	env := harness.NewEnv(t)
	process := startDaemon(env, env.Parachute("--schedule", "0 0 4 * * *"))
	next := announcedNext(t, process, 0)
	if utc := next.UTC(); utc.Hour() != 4 || utc.Minute() != 0 || utc.Second() != 0 {
		t.Errorf("next cycle %s, want 04:00:00 UTC", next)
	}
}

func TestSCH_002_invalid_cron_expression(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--schedule", "not cron").Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, "not cron")
}

func TestSCH_003_interval_and_schedule_are_exclusive(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--interval", "60", "--schedule", "0 0 4 * * *").Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, "interval", "schedule")
}

func TestSCH_004_time_zone_applies_to_schedule(t *testing.T) {
	env := harness.NewEnv(t)
	process := startDaemon(env, env.Parachute("--schedule", "0 0 4 * * *").Setenv("TZ", "Europe/Berlin"))
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	next := announcedNext(t, process, 0).In(berlin)
	if next.Hour() != 4 || next.Minute() != 0 || next.Second() != 0 {
		t.Errorf("next cycle %s, want 04:00:00 Europe/Berlin", next)
	}
}

func TestSCH_004_unknown_time_zone(t *testing.T) {
	env := harness.NewEnv(t)
	assertExit(t, env.Parachute("--run-once").Setenv("TZ", "Mars/Olympus").Run(runTimeout), 2)
}

func TestSCH_005_run_once_without_updates_exits_0(t *testing.T) {
	env := harness.NewEnv(t)
	env.RunContainer("a", fixture(env), harness.ContainerOptions{})
	result := env.Parachute("--run-once", "--schedule", "0 0 4 * * *").Run(runTimeout)
	assertExit(t, result, 0)
	if starts := cycleStarts(t, result.Logs()); len(starts) != 1 {
		t.Errorf("expected exactly one cycle, got %d", len(starts))
	}
}

func TestSCH_006_cycles_never_overlap(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	proxy := app.registry.Proxy(harness.ProxyOptions{Delay: 12 * time.Second})
	env.RunContainer("slow", env.Rehome(app.ref, proxy.Host), harness.ContainerOptions{})
	process := startDaemon(env, env.Parachute("--interval", "5"))
	time.Sleep(20 * time.Second)
	logs := process.Logs()
	var running string
	for _, entry := range logs {
		switch entry.Msg() {
		case "cycle started":
			if running != "" {
				t.Fatalf("cycle %s started while %s was running", entry.Get("cycle"), running)
			}
			running = entry.Get("cycle")
		case "cycle finished":
			running = ""
		}
	}
	skips := findLogs(logs, func(entry harness.Log) bool {
		return entry.Level() == "info" && entry.Get("running_cycle") != "" && entry.Msg() == "scheduled cycle skipped, previous cycle still running"
	})
	if len(skips) == 0 {
		t.Error("no skipped start was logged")
	}
}

func TestSCH_007_startup_message_is_sent(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	assertExit(t, env.Parachute("--run-once", "--notification-url", webhook.URL()).Run(runTimeout), 0)
	startup := webhook.Matching("Parachute started")
	if len(startup) != 1 {
		t.Fatalf("expected one startup message, got %v", webhook.Received())
	}
	assertContains(t, startup[0].Message, "version: ", "schedule: ", "armed: false")
}

func TestSCH_007_startup_message_can_be_suppressed(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	assertExit(t, env.Parachute("--run-once", "--no-startup-message", "--notification-url", webhook.URL()).Run(runTimeout), 0)
	if startup := webhook.Matching("Parachute started"); len(startup) != 0 {
		t.Fatalf("unexpected startup message %v", startup)
	}
}
