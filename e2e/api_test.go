package e2e

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestAPI_001_no_port_without_http_settings(t *testing.T) {
	env := harness.NewEnv(t)
	container := env.StartParachuteContainer("parachute", harness.ContainerRun{Args: []string{"--scope", env.Scope()}})
	env.Eventually(60*time.Second, "Parachute started", func() bool {
		return strings.Contains(env.Docker("logs", container), "Parachute started")
	})
	sockets := env.Docker("exec", container, "cat", "/proc/net/tcp", "/proc/net/tcp6")
	for _, line := range strings.Split(sockets, "\n") {
		if fields := strings.Fields(line); len(fields) > 3 && fields[3] == "0A" {
			t.Errorf("a TCP port is listening: %s", line)
		}
	}
}

func TestAPI_001_disabled_endpoint_returns_404(t *testing.T) {
	env := harness.NewEnv(t)
	_, api := startAPI(env, "--http-api-metrics")
	assertResponse(t, trigger(api, harness.Bearer(apiToken)), http.StatusNotFound)
}

func TestAPI_001_unsupported_method_returns_405(t *testing.T) {
	env := harness.NewEnv(t)
	_, api := startAPI(env, "--http-api-update")
	response := api.Do(harness.APIRequest{Method: http.MethodDelete, Path: "/v1/update", Authorization: harness.Bearer(apiToken)})
	assertResponse(t, response, http.StatusMethodNotAllowed)
}

func TestAPI_002_missing_or_wrong_token_is_rejected(t *testing.T) {
	env := harness.NewEnv(t)
	process, api := startAPI(env, "--http-api-update")
	for _, authorization := range []string{"", harness.Bearer("wrong")} {
		response := trigger(api, authorization)
		assertResponse(t, response, http.StatusUnauthorized)
		if response.Header.Get("WWW-Authenticate") == "" {
			t.Errorf("401 without WWW-Authenticate for %q", authorization)
		}
	}
	assertNoCycle(t, process)
}

func TestAPI_002_status_page_accepts_basic_credentials(t *testing.T) {
	env := harness.NewEnv(t)
	_, api := startAPI(env, "--http-api-status")
	assertResponse(t, api.Do(harness.APIRequest{Path: "/status", Authorization: harness.Basic("any", apiToken)}), http.StatusOK)
}

func TestAPI_002_update_rejects_basic_credentials(t *testing.T) {
	env := harness.NewEnv(t)
	process, api := startAPI(env, "--http-api-update")
	assertResponse(t, trigger(api, harness.Basic("any", apiToken)), http.StatusUnauthorized)
	assertNoCycle(t, process)
}

func TestAPI_002_status_token_opens_status_endpoints(t *testing.T) {
	env := harness.NewEnv(t)
	_, api := startAPI(env, "--http-api-status", "--http-api-status-token", "ro")
	assertResponse(t, api.Do(harness.APIRequest{Path: "/v1/status", Authorization: harness.Bearer("ro")}), http.StatusOK)
	assertResponse(t, api.Do(harness.APIRequest{Path: "/status", Authorization: harness.Basic("any", "ro")}), http.StatusOK)
}

func TestAPI_002_status_token_is_rejected_elsewhere(t *testing.T) {
	env := harness.NewEnv(t)
	process, api := startAPI(env, "--http-api-status-token", "ro", "--http-api-update", "--http-api-metrics")
	assertResponse(t, trigger(api, harness.Bearer("ro")), http.StatusUnauthorized)
	assertResponse(t, api.Do(harness.APIRequest{Path: "/v1/metrics", Authorization: harness.Bearer("ro")}), http.StatusUnauthorized)
	assertNoCycle(t, process)
}

func TestAPI_003_token_is_required(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--http-api-update").Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, "http-api-token")
}

func TestAPI_004_trigger_runs_one_cycle(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	container := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	_, api := startAPI(env, "--http-api-update")
	response := trigger(api, harness.Bearer(apiToken))
	assertResponse(t, response, http.StatusOK)
	body := response.JSON()
	if !regexp.MustCompile(`^cyc-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{6}$`).MatchString(body["cycle"].(string)) {
		t.Errorf("cycle ID %v", body["cycle"])
	}
	for key, want := range map[string]float64{"scanned": 1, "updated": 0, "failed": 0, "held": 0, "would_update": 1} {
		if body[key] != want {
			t.Errorf("%s = %v, want %v (body %s, container %s)", key, body[key], want, response.Body, container)
		}
	}
}

func TestAPI_004_query_parameter_is_rejected(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	container := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	id, image := env.ContainerState(container)
	app.publishNewer()
	process, api := startAPI(env, "--http-api-update")
	response := api.Do(harness.APIRequest{Method: http.MethodPost, Path: "/v1/update?stack=shop", Authorization: harness.Bearer(apiToken)})
	assertResponse(t, response, http.StatusBadRequest)
	if response.JSON()["error"] != "unknown parameter: stack" {
		t.Errorf("body %s", response.Body)
	}
	assertNoCycle(t, process)
	assertUnchanged(env, container, id, image)
}
