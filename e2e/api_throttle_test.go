package e2e

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func failAuthentication(t *testing.T, api *harness.API, count int, forwardedFor func(int) string) {
	t.Helper()
	for index := range count {
		response := api.Do(harness.APIRequest{Method: http.MethodPost, Path: "/v1/update", Authorization: harness.Bearer("wrong"), ForwardedFor: forwardedFor(index)})
		assertResponse(t, response, http.StatusUnauthorized)
	}
}

func noForwarding(int) string { return "" }

func forwardedTrigger(api *harness.API, forwardedFor string) *harness.APIResponse {
	return api.Do(harness.APIRequest{Method: http.MethodPost, Path: "/v1/update", Authorization: harness.Bearer(apiToken), ForwardedFor: forwardedFor})
}

func assertThrottled(t *testing.T, response *harness.APIResponse) {
	t.Helper()
	assertResponse(t, response, http.StatusTooManyRequests)
	retryAfter, err := strconv.Atoi(response.Header.Get("Retry-After"))
	if err != nil || retryAfter < 1 || retryAfter > 60 {
		t.Errorf("Retry-After %q", response.Header.Get("Retry-After"))
	}
}

func throttledDaemon(t *testing.T, args ...string) (*harness.Process, *harness.API) {
	t.Helper()
	env := harness.NewEnv(t)
	process, api := startAPI(env, append([]string{"--http-api-update"}, args...)...)
	started := time.Now()
	failAuthentication(t, api, 10, noForwarding)
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("10 failures took %s", elapsed)
	}
	return process, api
}

func TestAPI_010_repeated_failures_are_throttled(t *testing.T) {
	process, api := throttledDaemon(t)
	assertThrottled(t, trigger(api, harness.Bearer(apiToken)))
	assertNoCycle(t, process)
}

func TestAPI_010_parallel_failures_never_exceed_the_limit(t *testing.T) {
	env := harness.NewEnv(t)
	_, api := startAPI(env, "--http-api-update")
	statuses := make(chan int, 20)
	var requests sync.WaitGroup
	for range 20 {
		requests.Go(func() { statuses <- trigger(api, harness.Bearer("wrong")).Status })
	}
	requests.Wait()
	close(statuses)
	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[http.StatusUnauthorized] > 10 || counts[http.StatusUnauthorized]+counts[http.StatusTooManyRequests] != 20 {
		t.Errorf("parallel wrong tokens got %v, want at most 10 checked (401) and the rest 429", counts)
	}
}

func TestAPI_010_other_addresses_are_not_throttled(t *testing.T) {
	_, api := throttledDaemon(t)
	response := api.Do(harness.APIRequest{Method: http.MethodPost, Path: "/v1/update", Authorization: harness.Bearer(apiToken), Host: "::1"})
	assertResponse(t, response, http.StatusOK)
}

func TestAPI_010_throttle_ends_after_the_window(t *testing.T) {
	_, api := throttledDaemon(t)
	time.Sleep(65 * time.Second)
	assertResponse(t, trigger(api, harness.Bearer(apiToken)), http.StatusOK)
}

func TestAPI_010_forwarded_for_is_ignored_from_untrusted_peers(t *testing.T) {
	env := harness.NewEnv(t)
	_, api := startAPI(env, "--http-api-update")
	failAuthentication(t, api, 10, func(index int) string { return fmt.Sprintf("192.0.2.%d", index+1) })
	assertThrottled(t, forwardedTrigger(api, "192.0.2.99"))
}

func TestAPI_010_trusted_proxy_forwards_the_client_address(t *testing.T) {
	env := harness.NewEnv(t)
	process, api := startAPI(env, "--http-api-update", "--trusted-proxies", "127.0.0.1")
	failAuthentication(t, api, 10, func(int) string { return "192.0.2.10" })
	assertResponse(t, forwardedTrigger(api, "192.0.2.11"), http.StatusOK)
	assertThrottled(t, forwardedTrigger(api, "192.0.2.10"))
	naming := func(entry harness.Log) bool {
		return entry.Level() == "warn" && strings.Contains(entry.Raw, "192.0.2.10")
	}
	process.WaitForLogs(30*time.Second, "throttle warning", 1, naming)
	if warnings := findLogs(process.Logs(), naming); len(warnings) != 1 {
		t.Errorf("expected one warn line naming 192.0.2.10, got %d", len(warnings))
	}
}

func TestAPI_010_ipv6_sources_are_counted_per_64_prefix(t *testing.T) {
	env := harness.NewEnv(t)
	_, api := startAPI(env, "--http-api-update", "--trusted-proxies", "127.0.0.1")
	failAuthentication(t, api, 10, func(index int) string { return fmt.Sprintf("2001:db8:0:1::%x", index+1) })
	assertThrottled(t, forwardedTrigger(api, "2001:db8:0:1:ffff::1"))
	assertResponse(t, forwardedTrigger(api, "2001:db8:0:2::1"), http.StatusOK)
}

func TestAPI_010_oldest_source_is_evicted_when_the_table_is_full(t *testing.T) {
	env := harness.NewEnv(t)
	_, api := startAPI(env, "--http-api-update", "--trusted-proxies", "127.0.0.1")
	failAuthentication(t, api, 10, func(int) string { return "192.0.2.10" })
	failAuthentication(t, api, 10000, func(index int) string { return fmt.Sprintf("10.%d.%d.%d", index>>16, index>>8&255, index&255) })
	assertResponse(t, forwardedTrigger(api, "192.0.2.10"), http.StatusOK)
}

func TestAPI_010_invalid_trusted_proxy_is_startup_error(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--trusted-proxies", "not-an-ip").Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, "not-an-ip")
}

func TestAPI_011_wrong_tokens_of_any_shape_get_401(t *testing.T) {
	env := harness.NewEnv(t)
	token := strings.Repeat("k", 63) + "T"
	_, api := startAPI(env, "--http-api-update", "--http-api-status", "--http-api-token", token,
		"--http-api-status-token", "status-token", "--trusted-proxies", "127.0.0.1")
	wrong := []string{token[:63] + "U", strings.Repeat("z", 64), "", "q", strings.Repeat("w", 1<<10)}
	source := 0
	for _, presented := range wrong {
		for _, request := range []harness.APIRequest{
			{Method: http.MethodPost, Path: "/v1/update"},
			{Path: "/v1/status"},
		} {
			source++
			request.Authorization, request.ForwardedFor = "Bearer "+presented, fmt.Sprintf("192.0.2.%d", source)
			response := api.Do(request)
			if response.Status != http.StatusUnauthorized {
				t.Errorf("%s %s with a %d-character token: %d", request.Method, request.Path, len(presented), response.Status)
			}
		}
	}
}
