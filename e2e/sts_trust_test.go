package e2e

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestSTS_006_state_file_claims_do_not_arm_or_release_holds(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	held := env.RunContainer("held", app.ref, harness.ContainerOptions{Labels: map[string]string{"parachute.approval": "required"}})
	app.publishNewer()
	assertState(t, runOnce(env), held, "held", "approval_required")
	editState(t, env, func(document map[string]any) {
		var digest any
		for _, result := range document["results"].([]any) {
			entry := result.(map[string]any)
			if entry["container"] == held {
				digest = entry["candidate_digest"]
			}
		}
		now := time.Now().UTC().Format(time.RFC3339)
		document["approvals"] = []any{map[string]any{"host": "local", "container": held, "digest": digest, "time": now}}
		document["results"] = append(document["results"].([]any), map[string]any{
			"host": "local", "stack": "gone", "compose": false, "service": "gone", "container": "gone", "image": app.ref,
			"image_id": "sha256:0", "candidate_digest": "", "state": "updated", "reason": "", "txn": "txn-claimed", "snapshot": "snap-claimed",
		})
	})
	document := statusDocument(t, env)
	if document["armed"] != false {
		t.Errorf("armed %v", document["armed"])
	}
	hold, _ := statusServices(document)[held]["hold"].(map[string]any)
	if hold["reason"] != "approval_required" {
		t.Errorf("hold %v", hold)
	}
}

func TestSTS_006_label_strings_are_escaped(t *testing.T) {
	env := harness.NewEnv(t)
	service := "<script>alert(1)</script>"
	env.RunContainer("a", fixture(env), harness.ContainerOptions{Labels: composeLabels(env.Name("shop"), service)})
	process, api := startAPI(env, "--http-api-status")
	waitCycleFinished(process, 1)
	page := api.Do(harness.APIRequest{Path: "/status", Authorization: harness.Basic("any", apiToken)})
	assertResponse(t, page, http.StatusOK)
	assertContains(t, page.Body, "&lt;script&gt;alert(1)&lt;/script&gt;")
	if strings.Contains(strings.ToLower(page.Body), "<script") {
		t.Errorf("page contains a script element:\n%s", page.Body)
	}
	found := false
	for _, entry := range statusServices(apiStatus(t, api)) {
		found = found || entry["name"] == service
	}
	if !found {
		t.Errorf("GET /v1/status does not name service %q", service)
	}
}

func TestSTS_007_status_output_is_redacted(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.AuthRegistry("user", "s3cret-value")
	ref := registry.Push("app", "1", "1")
	env.WriteFile("docker-config/config.json", dockerConfig(t, map[string]string{registry.Host: "user:s3cret-value"}), 0o600)
	env.RunContainer("db", ref, harness.ContainerOptions{Extra: []string{"-e", "DB_PASSWORD=pw-9z7"}})
	registry.Push("app", "1", "2")
	settings := []string{"--http-api-token", "T0ken-api", "--http-api-status-token", "T0ken-ro",
		"--notification-url", "generic://bob:hunter2@127.0.0.1:1/hook?token=q5ecret"}
	runOnce(env, settings...)
	secrets := []string{"T0ken-api", "T0ken-ro", "hunter2", "q5ecret", "s3cret-value",
		base64.StdEncoding.EncodeToString([]byte("user:s3cret-value")), "pw-9z7"}
	for _, args := range [][]string{{"status"}, {"status", "--json"}} {
		result := env.Parachute(append(args, settings...)...).Run(runTimeout)
		assertExit(t, result, 0)
		for _, secret := range secrets {
			if strings.Contains(result.Stdout+result.Stderr, secret) {
				t.Errorf("%v output contains %q:\n%s\n%s", args, secret, result.Stdout, result.Stderr)
			}
		}
	}
}
