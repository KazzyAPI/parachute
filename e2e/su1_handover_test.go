package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func writeHandover(t *testing.T, env *harness.Env, marker map[string]any) {
	t.Helper()
	contents, err := json.Marshal(marker)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(env.StateDir, "handover.json"), contents, 0o600); err != nil {
		t.Fatal(err)
	}
}

func handoverChecksum(t *testing.T, env *harness.Env) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(env.StateDir, "handover.json"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:])
}

func imageRepoDigest(t *testing.T, env *harness.Env, ref string) string {
	t.Helper()
	line := env.Docker("inspect", "--format", "{{index .RepoDigests 0}}", ref)
	_, digest, ok := strings.Cut(strings.TrimSpace(line), "@")
	if !ok || digest == "" {
		t.Fatalf("repo digest for %s: %q", ref, line)
	}
	return digest
}

func TestUPD_018_invalid_handover_marker_exits_2_and_keeps_file(t *testing.T) {
	env := harness.NewEnv(t)
	ref := officialRef(env)
	startOwnWithStateMount(env, ref)
	before := handoverChecksum(t, env)
	writeHandover(t, env, map[string]any{
		"format": "parachute.handover/v1", "transaction": "not-a-txn", "container": strings.Repeat("a", 64), "digest": "sha256:deadbeef",
	})
	result := env.Parachute("--run-once").Run(runTimeout)
	assertExit(t, result, 2)
	if !strings.Contains(result.Stderr, "handover.json") {
		t.Fatalf("stderr %q", result.Stderr)
	}
	if after := handoverChecksum(t, env); after != before {
		t.Fatalf("checksum changed")
	}
}

func TestUPD_018_superseded_instance_exits_2_after_later_transaction(t *testing.T) {
	env := harness.NewEnv(t)
	ref := officialRef(env)
	own := startOwnWithStateMount(env, ref)
	oldID, _ := env.ContainerState(own)
	editState(t, env, func(document map[string]any) {
		document["superseded_instances"] = []any{oldID}
	})
	result := env.StartAttached(own, runTimeout)
	assertExit(t, result, 2)
}

func TestUPD_018_superseded_instance_exits_2_after_marker_removed(t *testing.T) {
	env := harness.NewEnv(t)
	ref := officialRef(env)
	own := startOwnWithStateMount(env, ref)
	oldID, _ := env.ContainerState(own)
	txn := "txn-20260101T000000Z-abc123"
	writeHandover(t, env, map[string]any{
		"format": "parachute.handover/v1", "transaction": txn, "container": oldID, "digest": "sha256:deadbeef",
	})
	editState(t, env, func(document map[string]any) {
		document["transactions"] = []any{map[string]any{
			"id": txn, "host": "local", "stacks": []any{env.Scope()}, "state": "committed",
			"self_update": true, "own_container": own, "result": "updated", "time": time.Now().UTC().Format(time.RFC3339),
		}}
	})
	result := env.StartAttached(own, runTimeout)
	assertExit(t, result, 2)
}

func startOwnWithStateMount(env *harness.Env, ref string) string {
	env.T.Helper()
	args := []string{"--run-once", "--porcelain", "v1", "--no-pull", "--scope", env.Scope(), "--state-dir", "/var/lib/parachute"}
	name := env.CreateParachuteContainer("parachute", ref, nil, harness.ContainerRun{
		Args:   args,
		Mounts: []string{env.StateDir + ":/var/lib/parachute"},
	})
	assertExit(env.T, env.StartAttached(name, runTimeout), 0)
	return name
}
