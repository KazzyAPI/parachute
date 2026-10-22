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
	assertExit(t, env.Parachute("status").Setenv("DOCKER_HOST", "tcp://127.0.0.1:1").Run(runTimeout), 4)
}

func TestSTS_002_document_shape(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	held := env.RunContainer("held", app.ref, harness.ContainerOptions{Labels: map[string]string{"parachute.monitor-only": "true"}})
	fresh := env.RunContainer("fresh", env.Registry().Push("other", "1", "1"), harness.ContainerOptions{})
	app.publishNewer()
	assertExit(t, runOnce(env), 0)
	document := statusDocument(t, env)
	for _, key := range []string{"format", "generated", "armed", "last_cycle", "hosts"} {
		if _, ok := document[key]; !ok {
			t.Errorf("document misses %s", key)
		}
	}
	services := statusServices(document)
	for _, key := range []string{"name", "container", "image", "image_id", "update_available", "candidate_digest", "hold", "drift", "last_transaction"} {
		if _, ok := services[fresh][key]; !ok {
			t.Errorf("service misses %s", key)
		}
	}
	if services[held]["hold"] == nil {
		t.Errorf("held service has no hold: %v", services[held])
	}
	if services[fresh]["hold"] != nil || services[fresh]["update_available"] != false {
		t.Errorf("fresh service: %v", services[fresh])
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
