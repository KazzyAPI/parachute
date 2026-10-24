package e2e

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestNTF_007_simple_mode_sends_the_cycle_log_messages(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	app := newUpdatable(env, "app")
	env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	result := notify(env, webhook, "--notification-report=false", "--notification-template", "{{range .}}{{.Message}}|{{end}}")
	var want strings.Builder
	inCycle := false
	for _, entry := range result.Logs() {
		inCycle = inCycle || entry.Msg() == "cycle started"
		if inCycle && slices.Contains([]string{"info", "warn", "error"}, entry.Level()) {
			want.WriteString(entry.Msg() + "|")
		}
		if entry.Msg() == "cycle finished" {
			break
		}
	}
	if body := singleReport(t, webhook).Message; body != want.String() {
		t.Errorf("body %q, want %q", body, want.String())
	}
}

func TestNTF_009_logger_target_writes_the_body_to_stdout(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	result := env.Parachute("--run-once", "--no-startup-message", "--notification-url", "logger://", "--notification-log-stdout").Run(runTimeout)
	assertExit(t, result, 0)
	assertContains(t, result.Stdout, a+"): would_update")
}

func TestNTF_011_approval_reminder_follows_reminder_days(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	db := newUpdatable(env, "postgres")
	name := env.RunContainer("db", db.ref, harness.ContainerOptions{})
	db.publishNewer()
	process := startDaemon(env, env.Parachute("--interval", "60", "--approval-reminder-days", "0.001",
		"--no-startup-message", "--notification-url", webhook.URL()))
	starts := process.WaitForLogs(5*time.Minute, "five cycles", 5, func(entry harness.Log) bool { return entry.Msg() == "cycle started" })
	process.WaitForLogs(time.Minute, "fifth cycle end", 5, func(entry harness.Log) bool { return entry.Msg() == "cycle finished" })
	env.Eventually(15*time.Second, "third reminder", func() bool { return len(webhook.Matching(name)) >= 3 })
	var cycles []int
	for _, report := range webhook.Matching(name) {
		cycle := 0
		for index, start := range starts {
			if !start.Time().After(report.Received) {
				cycle = index
			}
		}
		cycles = append(cycles, cycle)
	}
	if !slices.Equal(cycles, []int{0, 2, 4}) {
		t.Errorf("reports including %s came in cycles %v, want [0 2 4]", name, cycles)
	}
}

func TestNTF_011_zero_reminder_days_is_a_startup_error(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once", "--approval-reminder-days", "0").Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, "approval-reminder-days")
}

func TestNTF_011_delay_hold_is_not_repeated(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	process := startDaemon(env, env.Parachute("--interval", "5", "--delay-days", "2", "--no-startup-message", "--notification-url", webhook.URL()))
	process.WaitForLogs(time.Minute, "two cycles", 2, func(entry harness.Log) bool { return entry.Msg() == "cycle finished" })
	time.Sleep(3 * time.Second)
	received := webhook.Received()
	if len(received) != 1 {
		t.Fatalf("expected only the first cycle's notification, got %d: %v", len(received), received)
	}
	assertContains(t, received[0].Message, a, "held (delay)")
}

func TestNTF_014_notifications_carry_no_secrets(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	planted := plantSecrets(env)
	notify(env, webhook, append(planted.args, "--notification-report=false", "--notifications-level", "debug")...)
	if len(webhook.Received()) == 0 {
		t.Fatal("no notification received")
	}
	assertNotificationsHaveNoSecrets(t, webhook, planted.secrets...)
}

func TestNTF_014_realm_userinfo_is_redacted(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	host := challengeRegistry(env, func(string) string { return "https://u:realmpw@127.0.0.1:1/token" }, 0, "")
	challengedContainer(env, host)
	notify(env, webhook, "--notification-report=false")
	body := singleReport(t, webhook).Message
	assertContains(t, body, "[REDACTED]")
	assertNoSecrets(t, "notification", body, "realmpw")
}

func TestNTF_011_new_digest_is_reported_again(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	startDaemon(env, env.Parachute("--interval", "5", "--delay-days", "2", "--no-startup-message", "--notification-url", webhook.URL()))
	env.Eventually(time.Minute, "first hold report", func() bool { return len(webhook.Matching(a)) == 1 })
	app.registry.Push(refRepo(app.ref), "1", "3")
	env.Eventually(time.Minute, "hold report for the new digest", func() bool { return len(webhook.Matching(a)) == 2 })
}

func TestNTF_011_new_reason_is_reported_again(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	startDaemon(env, env.Parachute("--interval", "5", "--delay-days", "2", "--no-startup-message", "--notification-url", webhook.URL()))
	env.Eventually(time.Minute, "delay hold report", func() bool { return len(webhook.Matching(a)) == 1 })
	_, oldImage := env.ContainerState(a)
	env.Docker("rm", "-f", a)
	env.RunContainer("a", env.Tag(oldImage, app.ref), harness.ContainerOptions{Labels: map[string]string{"parachute.monitor-only": "true"}})
	env.Eventually(time.Minute, "monitor-only hold report", func() bool { return len(webhook.Matching("held (monitor_only)")) == 1 })
}

func TestNTF_014_template_functions_cannot_reveal_secrets(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	host := challengeRegistry(env, func(string) string { return "https://tok3nuser@127.0.0.1:1/token" }, 0, "")
	challengedContainer(env, host)
	notify(env, webhook, "--notification-report=false",
		"--notification-template", "{{range .}}{{urlquery .Message}};{{range $key, $value := .Fields}}{{urlquery $value}};{{end}}{{end}}")
	body := singleReport(t, webhook).Message
	assertContains(t, body, "REDACTED")
	assertNoSecrets(t, "notification", body, "tok3nuser")
}
