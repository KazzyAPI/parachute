package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func composeLabels(project, service string) map[string]string {
	return map[string]string{"com.docker.compose.project": project, "com.docker.compose.service": service}
}

func notify(env *harness.Env, webhook *harness.Webhook, args ...string) *harness.Result {
	env.T.Helper()
	return runOnce(env, append([]string{"--no-startup-message", "--notification-url", webhook.URL()}, args...)...)
}

func singleReport(t *testing.T, webhook *harness.Webhook) harness.Notification {
	t.Helper()
	reports := webhook.Reports()
	if len(reports) != 1 {
		t.Fatalf("expected exactly one report, got %d: %v", len(reports), reports)
	}
	return reports[0]
}

func TestNTF_001_every_url_receives_the_report(t *testing.T) {
	env := harness.NewEnv(t)
	first, second := env.Webhook(), env.Webhook()
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	notify(env, first, "--notification-url", second.URL())
	for _, webhook := range []*harness.Webhook{first, second} {
		assertContains(t, singleReport(t, webhook).Message, a)
	}
}

func TestNTF_001_failed_url_does_not_stop_others(t *testing.T) {
	env := harness.NewEnv(t)
	working := env.Webhook()
	app := newUpdatable(env, "app")
	env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	broken := "generic://user:hunter2@127.0.0.1:1/secret-path?disabletls=yes"
	result := notify(env, working, "--notification-url", broken)
	singleReport(t, working)
	errors := findLogs(result.Logs(), func(entry harness.Log) bool { return entry.Level() == "error" && entry.Get("host") == "127.0.0.1:1" })
	if len(errors) != 1 {
		t.Fatalf("expected one error line naming 127.0.0.1:1, got %d", len(errors))
	}
	for _, secret := range []string{"secret-path", "hunter2"} {
		if strings.Contains(result.Stderr, secret) {
			t.Errorf("logs contain %q", secret)
		}
	}
}

func TestNTF_002_one_digest_grouped_by_stack(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	app := newUpdatable(env, "app")
	shop, blog := env.Name("shop"), env.Name("blog")
	for _, project := range []string{shop, blog} {
		for _, service := range []string{"web", "worker"} {
			env.RunContainer(project+"-"+service, app.ref, harness.ContainerOptions{Labels: composeLabels(project, service)})
		}
	}
	app.publishNewer()
	notify(env, webhook)
	time.Sleep(time.Second)
	lines := strings.Split(singleReport(t, webhook).Message, "\n")
	for _, project := range []string{shop, blog} {
		index := indexOfLine(lines, "  "+project)
		if index < 0 || index+2 >= len(lines) {
			t.Fatalf("stack %s not listed:\n%s", project, strings.Join(lines, "\n"))
		}
		for offset, service := range []string{"web", "worker"} {
			if !strings.HasPrefix(lines[index+1+offset], "    "+service+" ") {
				t.Errorf("service %s not listed under %s:\n%s", service, project, strings.Join(lines, "\n"))
			}
		}
	}
}

func indexOfLine(lines []string, want string) int {
	for index, line := range lines {
		if line == want {
			return index
		}
	}
	return -1
}

func TestNTF_003_default_level_omits_fresh(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	app := newUpdatable(env, "app")
	updated := env.RunContainer("updated", app.ref, harness.ContainerOptions{})
	fresh := env.RunContainer("fresh", env.Registry().Push("other", "1", "1"), harness.ContainerOptions{})
	app.publishNewer()
	notify(env, webhook)
	report := singleReport(t, webhook).Message
	assertContains(t, report, updated)
	if strings.Contains(report, fresh) {
		t.Errorf("report lists the fresh container:\n%s", report)
	}
}

func TestNTF_003_warn_level_lists_only_failures(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	app := newUpdatable(env, "app")
	updated := env.RunContainer("updated", app.ref, harness.ContainerOptions{})
	failed := env.RunContainer("failed", env.Rehome(app.ref, "127.0.0.1:1"), harness.ContainerOptions{})
	app.publishNewer()
	notify(env, webhook, "--notifications-level", "warn")
	report := singleReport(t, webhook).Message
	assertContains(t, report, failed)
	if strings.Contains(report, updated) {
		t.Errorf("report lists the would_update container:\n%s", report)
	}
}

func TestNTF_003_invalid_level(t *testing.T) {
	env := harness.NewEnv(t)
	assertExit(t, env.Parachute("--notifications-level", "loud").Run(runTimeout), 2)
}

func TestNTF_004_title_with_tag_and_hostname(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	assertExit(t, env.Parachute("--run-once", "--notifications-hostname", "nas", "--notification-title-tag", "prod", "--notification-url", webhook.URL()).Run(runTimeout), 0)
	notification := webhook.WaitFor(10*time.Second, 1)[0]
	if notification.Title != "[prod] Parachute on nas" {
		t.Errorf("title %q", notification.Title)
	}
}

func TestNTF_004_skip_title(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	assertExit(t, env.Parachute("--run-once", "--notification-skip-title", "--notification-url", webhook.URL()).Run(runTimeout), 0)
	if notification := webhook.WaitFor(10*time.Second, 1)[0]; notification.HasTitle {
		t.Errorf("unexpected title %q", notification.Title)
	}
}

func TestNTF_005_report_is_delayed(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	app := newUpdatable(env, "app")
	env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	result := notify(env, webhook, "--notifications-delay", "10")
	finished := findLogs(result.Logs(), func(entry harness.Log) bool { return entry.Msg() == "cycle finished" })
	if len(finished) != 1 {
		t.Fatal("no cycle finished line")
	}
	assertDuration(t, singleReport(t, webhook).Received.Sub(finished[0].Time()), 11500*time.Millisecond, 1500*time.Millisecond)
}

func TestNTF_006_custom_template(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	app := newUpdatable(env, "app")
	env.RunContainer("a", app.ref, harness.ContainerOptions{Labels: composeLabels("a", "web")})
	env.RunContainer("b", app.ref, harness.ContainerOptions{Labels: composeLabels("b", "web")})
	app.publishNewer()
	notify(env, webhook, "--notification-template", "{{range .Hosts}}{{range .Stacks}}{{.Name}};{{end}}{{end}}")
	if body := singleReport(t, webhook).Message; body != "a;b;" {
		t.Errorf("body %q, want %q", body, "a;b;")
	}
}

func TestNTF_006_invalid_template(t *testing.T) {
	env := harness.NewEnv(t)
	assertExit(t, env.Parachute("--notification-template", "{{.Nope").Run(runTimeout), 2)
}

func TestNTF_008_all_fresh_sends_nothing(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	env.RunContainer("a", fixture(env), harness.ContainerOptions{})
	assertExit(t, notify(env, webhook), 0)
	time.Sleep(time.Second)
	if received := webhook.Received(); len(received) != 0 {
		t.Errorf("unexpected notifications %v", received)
	}
}

func TestNTF_010_dry_run_uses_the_report_path(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	notify(env, webhook)
	assertContains(t, singleReport(t, webhook).Message, a, "would_update")
}

func TestNTF_013_per_provider_settings_are_removed(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once").Setenv("PARACHUTE_NOTIFICATION_SLACK_HOOK_URL", "https://hooks.example/x").Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, "--notification-url", "slack://")
}
