package e2e

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestDMN_001_default_socket_is_host_local(t *testing.T) {
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skip("no Docker socket at the default path")
	}
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once").Unsetenv("DOCKER_HOST").Run(runTimeout)
	assertExit(t, result, 0)
	assertContains(t, startupLog(t, result.Logs()).Get("hosts"), "local=unix:///var/run/docker.sock")
}

func TestDMN_001_unsupported_scheme(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once").Setenv("DOCKER_HOST", "ssh://user@box").Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, "ssh")
}

func TestDMN_002_negotiated_version_completes_cycle(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once").Run(runTimeout)
	assertExit(t, result, 0)
	if len(findLogs(result.Logs(), func(entry harness.Log) bool { return entry.Msg() == "cycle finished" })) != 1 {
		t.Fatal("no completed cycle")
	}
}

func TestDMN_002_forced_old_version_exits_4(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once", "--api-version", "1.12").Run(runTimeout)
	assertExit(t, result, 4)
	assertContains(t, result.Stderr, "1.12", "too old")
}

func TestDMN_003_untrusted_daemon_certificate_exits_4(t *testing.T) {
	env := harness.NewEnv(t)
	daemon := env.FakeTLSDaemon()
	result := env.Parachute("--run-once", "--tlsverify").
		Setenv("DOCKER_HOST", daemon.Endpoint).Setenv("DOCKER_CERT_PATH", env.UnrelatedCertificates()).Run(runTimeout)
	assertExit(t, result, 4)
	if plain := daemon.PlainRequests(); plain > 0 {
		t.Errorf("daemon received %d plain-HTTP connections", plain)
	}
}

func TestDMN_003_insecure_tcp_host_exits_2(t *testing.T) {
	env := harness.NewEnv(t)
	proxy := env.EngineProxy()
	result := env.Parachute("--run-once").Setenv("DOCKER_HOST", proxy.Endpoint()).Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, proxy.Endpoint(), "allow-insecure-tcp")
	if connections := proxy.Connections(); connections > 0 {
		t.Errorf("daemon received %d connections", connections)
	}
}

func TestDMN_003_allowed_insecure_host_is_warned_every_cycle(t *testing.T) {
	env := harness.NewEnv(t)
	proxy := env.EngineProxy()
	process := startDaemon(env, env.Parachute("--allow-insecure-tcp", "--interval", "30").Setenv("DOCKER_HOST", proxy.Endpoint()))
	process.WaitForLogs(60*time.Second, "two cycles", 2, func(entry harness.Log) bool { return entry.Msg() == "cycle finished" })
	warnings := findLogs(process.Logs(), func(entry harness.Log) bool {
		return entry.Level() == "warn" && entry.Get("endpoint") == proxy.Endpoint()
	})
	if len(warnings) < 3 {
		t.Errorf("%d warn lines name %s, want at least 3", len(warnings), proxy.Endpoint())
	}
	assertContains(t, startupLog(t, process.Logs()).Get("relaxed"), "allow-insecure-tcp")
}

func TestDMN_003_insecure_host_in_host_list_exits_2(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once").Setenv("PARACHUTE_HOSTS", "a="+env.DockerHost()+",b=tcp://127.0.0.1:2375").Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, "tcp://127.0.0.1:2375")
}

func TestDMN_004_unreachable_host_exits_4(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once", "--allow-insecure-tcp").Setenv("DOCKER_HOST", "tcp://127.0.0.1:1").Run(runTimeout)
	assertExit(t, result, 4)
	assertContains(t, result.Stderr, "tcp://127.0.0.1:1")
}

func TestDMN_005_survives_daemon_outage_while_idle(t *testing.T) {
	env := harness.NewEnv(t)
	proxy := env.EngineProxy()
	process := startDaemon(env, env.Parachute("--allow-insecure-tcp", "--interval", "15").Setenv("DOCKER_HOST", proxy.Endpoint()))
	process.WaitForLog(30*time.Second, "first cycle", func(entry harness.Log) bool { return entry.Msg() == "cycle finished" })
	time.Sleep(2 * time.Second)
	proxy.Pause()
	time.Sleep(20 * time.Second)
	proxy.Resume()
	process.WaitForLogs(60*time.Second, "cycle after the outage", 2, func(entry harness.Log) bool { return entry.Msg() == "cycle finished" })
	if !process.Running() {
		t.Fatal("Parachute exited")
	}
}

func TestDMN_005_long_outage_reports_host_unreachable(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	proxy := env.EngineProxy()
	image := fixture(env)
	env.RunContainer("a", image, harness.ContainerOptions{})
	process := startDaemon(env, env.Parachute("--allow-insecure-tcp", "--interval", "30", "--notifications-level", "debug", "--notification-url", webhook.URL()).
		Setenv("DOCKER_HOST", proxy.Endpoint()))
	process.WaitForLog(30*time.Second, "first cycle", func(entry harness.Log) bool { return entry.Msg() == "cycle finished" })
	proxy.Pause()
	env.Eventually(8*time.Minute, "unreachable report", func() bool {
		for _, report := range webhook.Reports() {
			if strings.Contains(report.Message, "local: unreachable") {
				return true
			}
		}
		return false
	})
	if !process.Running() {
		t.Fatal("Parachute exited")
	}
}

func TestDMN_006_healthcheck_in_running_container(t *testing.T) {
	env := harness.NewEnv(t)
	container := env.StartParachuteContainer("parachute", harness.ContainerRun{Args: []string{"--scope", env.Scope()}})
	env.Eventually(90*time.Second, "healthy container", func() bool {
		status, _ := env.DockerMayFail("inspect", "--format", "{{.State.Health.Status}}", container)
		return status == "healthy"
	})
	if _, err := env.DockerMayFail("exec", container, "parachute", "healthcheck"); err != nil {
		t.Fatalf("healthcheck failed: %v", err)
	}
}

func TestDMN_006_healthcheck_without_daemon_exits_1(t *testing.T) {
	env := harness.NewEnv(t)
	container := env.Name("sleeper")
	env.Docker("run", "-d", "--name", container, "--label", "parachute-e2e="+env.ID, "--entrypoint", "sleep", env.ParachuteImage(), "infinity")
	out, err := env.DockerMayFail("exec", container, "parachute", "healthcheck")
	if err == nil || !strings.Contains(err.Error(), "exit status 1") {
		t.Fatalf("healthcheck: %v %s", err, out)
	}
}

func TestDMN_013_runtime_managed_container_is_skipped(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Labels: map[string]string{"io.containers.autoupdate": "registry"}})
	app.publishNewer()
	id, image := env.ContainerState(a)
	assertState(t, armedOnce(env), a, "skipped", "managed_by_runtime")
	assertUnchanged(env, a, id, image)
}
