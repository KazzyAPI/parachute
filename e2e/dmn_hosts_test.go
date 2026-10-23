package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestDMN_005_engine_error_fails_only_that_candidate(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	a := env.RunContainer("a", registry.Push("a", "1", "1"), harness.ContainerOptions{})
	b := env.RunContainer("b", registry.Push("b", "1", "2"), harness.ContainerOptions{})
	_, imageA := env.ContainerState(a)
	inspectA := "/images/" + imageA + "/json"
	proxy := env.HTTPEngineProxy()
	proxy.Fail(inspectA)
	result := env.Parachute("--run-once", "--porcelain", "v1", "--allow-insecure-tcp").Setenv("DOCKER_HOST", proxy.Endpoint).Run(runTimeout)
	assertExit(t, result, 1)
	assertState(t, result, a, "failed", "engine_error")
	assertState(t, result, b, "fresh", "")
	if unreachable := findLogs(result.Logs(), func(entry harness.Log) bool { return entry.Msg() == "host unreachable" }); len(unreachable) > 0 {
		t.Errorf("host reported unreachable: %v", unreachable[0].Raw)
	}
	inspections := 0
	for _, request := range proxy.Requests() {
		if strings.HasSuffix(request, inspectA) {
			inspections++
		}
	}
	if inspections != 1 {
		t.Errorf("%d inspect requests for %s, want 1 (engine errors are not retried)", inspections, imageA)
	}
}

func TestDMN_007_host_list_is_in_startup_message(t *testing.T) {
	env := harness.NewEnv(t)
	process := startDaemon(env, env.Parachute("--allow-insecure-tcp").
		Setenv("PARACHUTE_HOSTS", "a="+env.DockerHost()+",b=tcp://10.0.0.5:2376"))
	started := process.WaitForLog(90*time.Second, "startup message", func(entry harness.Log) bool { return entry.Msg() == "Parachute started" })
	assertContains(t, started.Get("hosts"), "a="+env.DockerHost(), "b=tcp://10.0.0.5:2376")
}

func TestDMN_007_duplicate_host_name_exits_2(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once").Setenv("PARACHUTE_HOSTS", "a="+env.DockerHost()+",a="+env.DockerHost()).Run(runTimeout)
	assertExit(t, result, 2)
}

func TestDMN_007_same_engine_under_two_names_exits_2(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once").Setenv("PARACHUTE_HOSTS", "a="+env.DockerHost()+",b="+env.DockerHost()).Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, "hosts a and b")
}

func TestDMN_008_per_host_certificates(t *testing.T) {
	env := harness.NewEnv(t)
	a := env.TLSEngineProxy("certs/a")
	b := env.TLSEngineProxy("certs/b")
	result := env.Parachute("--run-once", "--porcelain", "v1", "--tlsverify").
		Setenv("DOCKER_CERT_PATH", env.CertDir("certs")).
		Setenv("PARACHUTE_HOSTS", "a="+a.Endpoint+",b="+b.Endpoint).Run(runTimeout)
	assertExit(t, result, 0)
	for name, proxy := range map[string]*harness.HTTPEngineProxy{"a": a, "b": b} {
		if len(proxy.Requests()) == 0 {
			t.Errorf("host %s received no request", name)
		}
	}
}

func TestDMN_009_unreachable_host_does_not_stop_others(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	result := env.Parachute("--run-once", "--porcelain", "v1", "--allow-insecure-tcp", "--notifications-level", "debug",
		"--notification-url", webhook.URL()).
		Setenv("PARACHUTE_HOSTS", "a="+env.DockerHost()+",b=tcp://127.0.0.1:1").Run(runTimeout)
	assertExit(t, result, 1)
	entry := assertState(t, result, a, "would_update", "")
	if entry["host"] != "a" {
		t.Errorf("%s reported on host %v, want a", a, entry["host"])
	}
	env.Eventually(30*time.Second, "report naming b unreachable", func() bool {
		for _, report := range webhook.Reports() {
			if strings.Contains(report.Message, "b: unreachable") {
				return true
			}
		}
		return false
	})
}
