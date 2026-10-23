package e2e

import (
	"net/http"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestAPI_005_overlapping_trigger_returns_409(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	proxy := app.registry.Proxy(harness.ProxyOptions{Delay: 12 * time.Second})
	env.RunContainer("slow", env.Rehome(app.ref, proxy.Host), harness.ContainerOptions{})
	process, api := startAPI(env, "--http-api-update", "--http-api-metrics")
	first := make(chan *harness.APIResponse, 1)
	go func() {
		response, err := api.Try(harness.APIRequest{Method: http.MethodPost, Path: "/v1/update", Authorization: harness.Bearer(apiToken)})
		if err != nil {
			t.Error(err)
		}
		first <- response
	}()
	running := process.WaitForLog(30*time.Second, "first cycle", func(entry harness.Log) bool { return entry.Msg() == "cycle started" }).Get("cycle")
	sent := time.Now()
	second := trigger(api, harness.Bearer(apiToken))
	if elapsed := time.Since(sent); elapsed > time.Second {
		t.Errorf("409 took %s", elapsed)
	}
	assertResponse(t, second, http.StatusConflict)
	if second.JSON()["running"] != running {
		t.Errorf("body %s, want running %s", second.Body, running)
	}
	if response := <-first; response != nil {
		assertResponse(t, response, http.StatusOK)
	}
	metrics := api.Do(harness.APIRequest{Path: "/v1/metrics", Authorization: harness.Bearer(apiToken)})
	assertContains(t, metrics.Body, "parachute_scans_skipped 1\n")
}

func TestAPI_006_periodic_polls_run_alongside_the_api(t *testing.T) {
	env := harness.NewEnv(t)
	started := time.Now()
	process, api := startAPI(env, "--http-api-update", "--http-api-periodic-polls", "--interval", "30")
	waitCycleFinished(process, 1)
	assertResponse(t, trigger(api, harness.Bearer(apiToken)), http.StatusOK)
	time.Sleep(time.Until(started.Add(40 * time.Second)))
	if cycles := cyclesStarted(process); cycles < 2 {
		t.Errorf("%d cycles ran, want at least 2", cycles)
	}
	assertNoOverlap(t, process.Logs())
}

func TestAPI_008_metrics_describe_the_last_cycle(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	env.RunContainer("a", app.ref, harness.ContainerOptions{})
	other := fixture(env)
	env.RunContainer("b", other, harness.ContainerOptions{})
	env.RunContainer("c", other, harness.ContainerOptions{})
	app.publishNewer()
	process, api := startAPI(env, "--http-api-metrics")
	waitCycleFinished(process, 1)
	response := api.Do(harness.APIRequest{Path: "/v1/metrics", Authorization: harness.Bearer(apiToken)})
	assertResponse(t, response, http.StatusOK)
	assertContains(t, response.Body,
		"parachute_containers_scanned{host=\"local\"} 3\n",
		"parachute_containers_updated{host=\"local\"} 0\n",
		"parachute_scans_total 1\n")
}

func TestAPI_008_unreachable_host_is_reported_down(t *testing.T) {
	t.Skip("skipped until the hosts package (feat/m2-hosts, --hosts) merges; see M2 deviations")
	env := harness.NewEnv(t)
	process, api := startAPI(env, "--http-api-metrics", "--allow-insecure-tcp",
		"--hosts", "local="+env.DockerHost()+",remote=tcp://127.0.0.1:1")
	waitCycleFinished(process, 1)
	response := api.Do(harness.APIRequest{Path: "/v1/metrics", Authorization: harness.Bearer(apiToken)})
	assertContains(t, response.Body, "parachute_host_up{host=\"remote\"} 0\n")
}
