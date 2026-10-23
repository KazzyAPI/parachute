package e2e

import (
	"net/http"
	"net/url"
	"reflect"
	"syscall"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func approve(api *harness.API, query url.Values) *harness.APIResponse {
	return api.Do(harness.APIRequest{Method: http.MethodPost, Path: "/v1/approve?" + query.Encode(), Authorization: harness.Bearer(apiToken)})
}

func heldDatabase(t *testing.T, env *harness.Env) (string, updatable) {
	t.Helper()
	database := newUpdatable(env, "postgres")
	container := env.RunContainer("db", database.ref, harness.ContainerOptions{})
	database.publishNewer()
	return container, database
}

func stopDaemon(process *harness.Process) {
	process.Signal(syscall.SIGTERM)
	process.Wait(30 * time.Second)
}

func TestAPI_007_approved_update_survives_a_restart(t *testing.T) {
	env := harness.NewEnv(t)
	db, _ := heldDatabase(t, env)
	process, api := startAPI(env, "--http-api-update", "--porcelain", "v1")
	assertResponse(t, trigger(api, harness.Bearer(apiToken)), http.StatusOK)
	held := porcelainEntry(process, db)
	response := approve(api, url.Values{"container": {db}})
	assertResponse(t, response, http.StatusAccepted)
	if response.JSON()["digest"] != held["candidate_digest"] {
		t.Errorf("approval digest %v, held candidate %v", response.JSON()["digest"], held["candidate_digest"])
	}
	stopDaemon(process)
	process, api = startAPI(env, "--http-api-update", "--porcelain", "v1")
	assertResponse(t, trigger(api, harness.Bearer(apiToken)), http.StatusOK)
	if entry := porcelainEntry(process, db); entry["state"] != "would_update" {
		t.Errorf("after approval and restart %s is %v", db, entry)
	}
}

func TestAPI_007_approval_lapses_when_the_digest_changes(t *testing.T) {
	env := harness.NewEnv(t)
	db, database := heldDatabase(t, env)
	id, image := env.ContainerState(db)
	process, api := startAPI(env, "--http-api-update", "--porcelain", "v1")
	assertResponse(t, trigger(api, harness.Bearer(apiToken)), http.StatusOK)
	approved := porcelainEntry(process, db)["candidate_digest"]
	assertResponse(t, approve(api, url.Values{"container": {db}}), http.StatusAccepted)
	stopDaemon(process)
	database.registry.Push("postgres", "1", "3")
	process, api = startAPI(env, "--http-api-update", "--porcelain", "v1")
	assertResponse(t, trigger(api, harness.Bearer(apiToken)), http.StatusOK)
	entry := porcelainEntry(process, db)
	if entry["state"] != "held" || entry["reason"] != "approval_required" || entry["candidate_digest"] == approved {
		t.Errorf("after a new digest %s is %v, approved digest was %v", db, entry, approved)
	}
	assertUnchanged(env, db, id, image)
}

func TestAPI_007_container_without_held_update_is_404(t *testing.T) {
	env := harness.NewEnv(t)
	web := env.RunContainer("web", fixture(env), harness.ContainerOptions{})
	_, api := startAPI(env, "--http-api-update")
	assertResponse(t, trigger(api, harness.Bearer(apiToken)), http.StatusOK)
	assertResponse(t, approve(api, url.Values{"container": {web}}), http.StatusNotFound)
}

func TestAPI_007_unknown_container_is_404(t *testing.T) {
	env := harness.NewEnv(t)
	_, api := startAPI(env, "--http-api-update")
	assertResponse(t, approve(api, url.Values{"container": {"ghost"}}), http.StatusNotFound)
}

func TestAPI_007_request_must_name_container_or_stack(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	held := env.RunContainer("held", app.ref, harness.ContainerOptions{Labels: map[string]string{"parachute.monitor-only": "true"}})
	app.publishNewer()
	_, api := startAPI(env, "--http-api-update", "--http-api-status")
	assertResponse(t, trigger(api, harness.Bearer(apiToken)), http.StatusOK)
	before := apiStatus(t, api)
	assertResponse(t, approve(api, url.Values{"container": {held}, "stack": {"shop"}}), http.StatusBadRequest)
	assertResponse(t, approve(api, url.Values{}), http.StatusBadRequest)
	after := apiStatus(t, api)
	if !reflect.DeepEqual(before["hosts"], after["hosts"]) {
		t.Errorf("holds or freezes changed:\nbefore %v\nafter  %v", before["hosts"], after["hosts"])
	}
	if statusServices(after)[held]["hold"] == nil {
		t.Errorf("%s is not held: %v", held, statusServices(after)[held])
	}
}
