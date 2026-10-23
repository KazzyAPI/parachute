package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

const composeConfigFilesLabel = "com.docker.compose.project.config_files"

func assertKeys(t *testing.T, what string, object any, keys ...string) map[string]any {
	t.Helper()
	fields, ok := object.(map[string]any)
	if !ok {
		t.Fatalf("%s is not an object: %v", what, object)
	}
	for _, key := range keys {
		if _, present := fields[key]; !present {
			t.Errorf("%s misses %s: %v", what, key, fields)
		}
	}
	return fields
}

func statusStacks(document map[string]any) map[string]map[string]any {
	stacks := map[string]map[string]any{}
	for _, host := range document["hosts"].([]any) {
		for _, stack := range host.(map[string]any)["stacks"].([]any) {
			entry := stack.(map[string]any)
			stacks[entry["name"].(string)] = entry
		}
	}
	return stacks
}

func editState(t *testing.T, env *harness.Env, change func(document map[string]any)) {
	t.Helper()
	document := env.StateDocument()
	change(document)
	contents, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(env.StateDir, "state.json"), contents, 0o600); err != nil {
		t.Fatal(err)
	}
}
