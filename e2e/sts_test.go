package e2e

import (
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func statusDocument(t *testing.T, env *harness.Env) map[string]any {
	t.Helper()
	result := env.Parachute("status", "--json").Run(runTimeout)
	assertExit(t, result, 0)
	var document map[string]any
	if err := json.Unmarshal([]byte(result.Stdout), &document); err != nil {
		t.Fatalf("status is not JSON: %v\n%s", err, result.Stdout)
	}
	return document
}

func statusServices(document map[string]any) map[string]map[string]any {
	services := map[string]map[string]any{}
	for _, host := range document["hosts"].([]any) {
		for _, stack := range host.(map[string]any)["stacks"].([]any) {
			for _, service := range stack.(map[string]any)["services"].([]any) {
				entry := service.(map[string]any)
				services[entry["container"].(string)] = entry
			}
		}
	}
	return services
}

func TestSTS_001_status_without_daemon(t *testing.T) {
	env := harness.NewEnv(t)
	a := env.RunContainer("a", fixture(env), harness.ContainerOptions{})
	assertExit(t, runOnce(env), 0)
	document := statusDocument(t, env)
	if document["format"] != "parachute.status/v1" {
		t.Errorf("format %v", document["format"])
	}
	if _, listed := statusServices(document)[a]; !listed {
		t.Errorf("status does not list %s: %v", a, document)
	}
}

func TestSTS_001_status_with_unreachable_host_exits_4(t *testing.T) {
	env := harness.NewEnv(t)
	assertExit(t, env.Parachute("status", "--allow-insecure-tcp").Setenv("DOCKER_HOST", "tcp://127.0.0.1:1").Run(runTimeout), 4)
}

func TestSTS_001_status_with_insecure_host_exits_2(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("status").Setenv("DOCKER_HOST", "tcp://127.0.0.1:1").Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, "allow-insecure-tcp")
}

func TestSTS_002_held_and_fresh_services_in_document_shape(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	stack := env.Name("shop")
	held := env.RunContainer("held", app.ref, harness.ContainerOptions{Labels: composeLabels(stack, "web")})
	fresh := env.RunContainer("fresh", env.Registry().Push("other", "1", "1"), harness.ContainerOptions{Labels: composeLabels(stack, "cache")})
	app.publishNewer()
	process, api := startAPI(env, "--http-api-status", "--delay-days", "1")
	waitCycleFinished(process, 1)
	document := apiStatus(t, api)
	if document["format"] != "parachute.status/v1" {
		t.Errorf("format %v", document["format"])
	}
	assertKeys(t, "document", document, "format", "generated", "armed", "last_cycle", "hosts")
	assertKeys(t, "last_cycle", document["last_cycle"], "id", "started", "ended")
	for _, host := range document["hosts"].([]any) {
		assertKeys(t, "host", host, "name", "reachable", "stacks")
	}
	entry := assertKeys(t, "stack", statusStacks(document)[stack], "name", "compose", "last_backup", "last_restore_verification",
		"last_good_restore_age_seconds", "frozen", "services")
	if frozen, ok := entry["frozen"].([]any); !ok || len(frozen) != 0 || entry["compose"] != true {
		t.Errorf("stack %v", entry)
	}
	services := statusServices(document)
	for _, name := range []string{held, fresh} {
		assertKeys(t, name, services[name], "name", "container", "image", "image_id", "update_available", "candidate_digest",
			"hold", "drift", "last_transaction")
	}
	hold := assertKeys(t, "hold", services[held]["hold"], "reason", "until")
	if hold["reason"] != "delay" || hold["until"] == nil {
		t.Errorf("held service hold %v", hold)
	}
	if services[fresh]["hold"] != nil || services[fresh]["update_available"] != false {
		t.Errorf("fresh service: %v", services[fresh])
	}
}

func TestSTS_002_unarmed_daemon_reports_armed_false(t *testing.T) {
	env := harness.NewEnv(t)
	env.RunContainer("a", fixture(env), harness.ContainerOptions{})
	_, api := startAPI(env, "--http-api-status")
	if armed := apiStatus(t, api)["armed"]; armed != false {
		t.Errorf("armed %v", armed)
	}
}

func TestSTS_002_status_process_computes_armed_from_its_own_environment(t *testing.T) {
	env := harness.NewEnv(t)
	env.RunContainer("a", fixture(env), harness.ContainerOptions{})
	result := env.Parachute("status", "--json", "--helper-image", env.ParachuteImage()).Setenv("PARACHUTE_ARMED", "true").Run(runTimeout)
	assertExit(t, result, 0)
	if armed := parseStatusJSON(t, result.Stdout)["armed"]; armed != true {
		t.Errorf("armed %v", armed)
	}
}

func TestSTS_005_status_is_read_only(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	env.RunContainer("a", registry.Push("app", "1", "1"), harness.ContainerOptions{})
	assertExit(t, runOnce(env), 0)
	statePath := filepath.Join(env.StateDir, "state.json")
	checksum := func() [32]byte {
		contents, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatal(err)
		}
		return sha256.Sum256(contents)
	}
	containers := func() string {
		return env.Docker("ps", "-a", "--no-trunc", "--filter", "label=parachute-e2e="+env.ID, "--format", "{{.ID}} {{.Image}} {{.State}}")
	}
	before, listed, requests := checksum(), containers(), len(registry.ParachuteRequests())
	for range 10 {
		statusDocument(t, env)
	}
	if checksum() != before {
		t.Error("state file changed")
	}
	if containers() != listed {
		t.Error("containers changed")
	}
	if after := len(registry.ParachuteRequests()); after != requests {
		t.Errorf("status contacted the registry %d times", after-requests)
	}
}
