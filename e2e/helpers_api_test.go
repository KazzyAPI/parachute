package e2e

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

const apiToken = "e2e-api-token"

func startAPI(env *harness.Env, args ...string) (*harness.Process, *harness.API) {
	env.T.Helper()
	return startAPIWith(env, func(parachute *harness.Parachute) *harness.Parachute { return parachute }, args...)
}

func startAPIWith(env *harness.Env, adjust func(*harness.Parachute) *harness.Parachute, args ...string) (*harness.Process, *harness.API) {
	env.T.Helper()
	port := env.FreePort()
	flags := []string{"--http-api-port", strconv.Itoa(port)}
	if !hasFlag(args, "--http-api-token") {
		flags = append(flags, "--http-api-token", apiToken)
	}
	process := startDaemon(env, adjust(env.Parachute(append(flags, args...)...)))
	api := env.API(port)
	api.WaitReady(60 * time.Second)
	return process, api
}

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag || strings.HasPrefix(arg, flag+"=") {
			return true
		}
	}
	return false
}

func trigger(api *harness.API, authorization string) *harness.APIResponse {
	return api.Do(harness.APIRequest{Method: http.MethodPost, Path: "/v1/update", Authorization: authorization})
}

func assertResponse(t *testing.T, response *harness.APIResponse, code int) {
	t.Helper()
	if response.Status != code {
		t.Fatalf("status %d, want %d\n%s", response.Status, code, response.Body)
	}
}

func cyclesStarted(process *harness.Process) int {
	return len(findLogs(process.Logs(), func(entry harness.Log) bool { return entry.Msg() == "cycle started" }))
}

func assertNoCycle(t *testing.T, process *harness.Process) {
	t.Helper()
	if started := cyclesStarted(process); started != 0 {
		t.Errorf("%d cycles started, want none", started)
	}
}

func assertNoOverlap(t *testing.T, logs []harness.Log) {
	t.Helper()
	running := ""
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
}

func waitCycleFinished(process *harness.Process, count int) {
	process.WaitForLogs(2*time.Minute, "finished cycle", count, func(entry harness.Log) bool { return entry.Msg() == "cycle finished" })
}

func porcelainEntry(process *harness.Process, container string) map[string]any {
	var found map[string]any
	for _, line := range process.Stdout() {
		if line.Get("type") == "entry" && line.Get("container") == container {
			found = line.Fields
		}
	}
	return found
}

func apiStatus(t *testing.T, api *harness.API) map[string]any {
	t.Helper()
	response := api.Do(harness.APIRequest{Path: "/v1/status", Authorization: harness.Bearer(apiToken)})
	assertResponse(t, response, http.StatusOK)
	document := response.JSON()
	if document == nil {
		t.Fatalf("status is not JSON: %s", response.Body)
	}
	return document
}
