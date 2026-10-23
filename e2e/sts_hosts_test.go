package e2e

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func reachability(document map[string]any) map[string]any {
	hosts := map[string]any{}
	for _, host := range document["hosts"].([]any) {
		entry := host.(map[string]any)
		hosts[entry["name"].(string)] = entry["reachable"]
	}
	return hosts
}

func assertPartlyReachable(t *testing.T, document map[string]any) {
	t.Helper()
	if hosts := reachability(document); hosts["a"] != true || hosts["b"] != false {
		t.Errorf("host reachability %v", hosts)
	}
}

func TestSTS_001_status_lists_unreachable_host_when_another_is_reachable(t *testing.T) {
	env := harness.NewEnv(t)
	env.RunContainer("a", fixture(env), harness.ContainerOptions{})
	hosts := "a=" + env.DockerHost() + ",b=tcp://127.0.0.1:1"
	result := env.Parachute("status", "--json", "--allow-insecure-tcp").Setenv("PARACHUTE_HOSTS", hosts).Run(runTimeout)
	assertExit(t, result, 0)
	var document map[string]any
	if err := json.Unmarshal([]byte(result.Stdout), &document); err != nil {
		t.Fatalf("status is not JSON: %v\n%s", err, result.Stdout)
	}
	assertPartlyReachable(t, document)
	_, api := startAPIWith(env, func(parachute *harness.Parachute) *harness.Parachute {
		return parachute.Setenv("PARACHUTE_HOSTS", hosts)
	}, "--http-api-status", "--allow-insecure-tcp")
	assertPartlyReachable(t, apiStatus(t, api))
	assertResponse(t, api.Do(harness.APIRequest{Path: "/status", Authorization: harness.Basic("any", apiToken)}), http.StatusOK)
}
