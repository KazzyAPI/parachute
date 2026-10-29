package e2e

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

type frozenShop struct {
	env         *harness.Env
	failed      string
	sibling     string
	siblingApp  updatable
	project     string
	transaction string
}

func freezeShop(t *testing.T, env *harness.Env, extra ...string) frozenShop {
	t.Helper()
	signalDir := filepath.Join(env.Dir, "signal")
	if err := os.MkdirAll(signalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	project := env.ID + "-shop"
	app := newUpdatable(env, "app")
	failed := env.RunContainer("a", app.ref, harness.ContainerOptions{
		Extra:  []string{"-v", signalDir + ":" + signalDir},
		Labels: map[string]string{"com.docker.compose.project": project, "com.docker.compose.service": "a", "parachute.verify.cmd": verifyFailLabels(signalDir)["parachute.verify.cmd"]},
	})
	verifyFailUpdate(env, "app")
	other := newUpdatable(env, "other")
	sibling := env.RunContainer("b", other.ref, harness.ContainerOptions{
		Labels: map[string]string{"com.docker.compose.project": project, "com.docker.compose.service": "b"},
	})
	done := runArmedAsync(env, append([]string{"--backup", "none"}, extra...)...)
	original := waitRenamedOriginal(env, failed)
	env.Docker("rm", "-f", original)
	if err := os.WriteFile(filepath.Join(signalDir, "go"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := <-done
	entry := assertState(t, result, failed, "rollback_failed", "restore_failed")
	transaction, _ := entry["txn"].(string)
	if !strings.HasPrefix(transaction, "txn-") {
		t.Fatalf("rollback_failed entry has no transaction: %v", entry)
	}
	return frozenShop{env: env, failed: failed, sibling: sibling, siblingApp: other, project: project, transaction: transaction}
}

func (s frozenShop) publishSibling(version string) {
	s.siblingApp.registry.Push("other", "1", version)
}

func (s frozenShop) update(t *testing.T) *harness.Result {
	t.Helper()
	return runArmed(s.env, "--backup", "none")
}

func TestPOL_010_rolled_back_digest_is_held_until_a_new_digest(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.registry.PushSpec("app", "1", harness.ImageSpec{Verify: true, Version: "2", Env: map[string]string{"EXIT_AFTER": "1"}})
	first := armedOnce(env, "--verify-stable", "3s")
	failed := assertState(t, first, a, "rolled_back", "verify_failed")
	second := armedOnce(env, "--verify-stable", "3s")
	held := assertState(t, second, a, "held", "previous_attempt_failed")
	if held["candidate_digest"] != failed["candidate_digest"] {
		t.Errorf("held digest %v, failed digest %v", held["candidate_digest"], failed["candidate_digest"])
	}
	app.registry.Push("app", "1", "3")
	assertState(t, armedOnce(env, "--verify-stable", "3s"), a, "updated", "")
}

func TestPOL_010_approval_retries_a_rolled_back_digest(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.registry.PushSpec("app", "1", harness.ImageSpec{Verify: true, Version: "2", Env: map[string]string{"EXIT_AFTER": "1"}})
	assertState(t, armedOnce(env, "--verify-stable", "3s"), a, "rolled_back", "verify_failed")
	assertState(t, armedOnce(env, "--verify-stable", "3s"), a, "held", "previous_attempt_failed")
	process, api := startAPI(env, "--http-api-update")
	assertResponse(t, approve(api, url.Values{"container": {a}}), http.StatusAccepted)
	stopDaemon(process)
	assertState(t, armedOnce(env, "--verify-stable", "3s"), a, "rolled_back", "verify_failed")
	assertState(t, armedOnce(env, "--verify-stable", "3s"), a, "held", "previous_attempt_failed")
}

func TestPOL_010_rollback_failed_freezes_the_stack_whatever_the_digest(t *testing.T) {
	env := harness.NewEnv(t)
	shop := freezeShop(t, env)
	shop.publishSibling("2")
	first := assertState(t, shop.update(t), shop.sibling, "held", "previous_attempt_failed")
	shop.publishSibling("3")
	second := assertState(t, shop.update(t), shop.sibling, "held", "previous_attempt_failed")
	if first["candidate_digest"] == second["candidate_digest"] {
		t.Errorf("the candidate digest did not change: %v", first["candidate_digest"])
	}
}

func TestPOL_010_release_ends_the_freeze(t *testing.T) {
	env := harness.NewEnv(t)
	shop := freezeShop(t, env)
	shop.publishSibling("2")
	assertState(t, shop.update(t), shop.sibling, "held", "previous_attempt_failed")
	process, api := startAPI(env, "--http-api-update")
	response := approve(api, url.Values{"container": {shop.sibling}})
	assertResponse(t, response, http.StatusOK)
	stopDaemon(process)
	assertState(t, shop.update(t), shop.sibling, "updated", "")
}

func TestPOL_011_release_record_must_match_the_freeze(t *testing.T) {
	env := harness.NewEnv(t)
	shop := freezeShop(t, env)
	shop.publishSibling("2")
	path := filepath.Join(env.StateDir, "state.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(contents, &document); err != nil {
		t.Fatal(err)
	}
	document["releases"] = []any{map[string]any{
		"transaction": shop.transaction,
		"keys":        []any{map[string]any{"host": "local", "stack": "unrelated"}},
		"time":        time.Now().UTC().Format(time.RFC3339),
	}}
	contents, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	assertState(t, shop.update(t), shop.sibling, "held", "previous_attempt_failed")
}

func TestAPI_007_release_by_container_reports_the_transactions(t *testing.T) {
	env := harness.NewEnv(t)
	shop := freezeShop(t, env)
	_, api := startAPI(env, "--http-api-update")
	response := approve(api, url.Values{"container": {shop.sibling}})
	assertResponse(t, response, http.StatusOK)
	body := response.JSON()
	if body["container"] != shop.sibling || body["host"] != "local" {
		t.Errorf("release body %v", body)
	}
	if transactions, _ := body["transactions"].([]any); len(transactions) != 1 || transactions[0] != shop.transaction {
		t.Errorf("released transactions %v, want [%s]", body["transactions"], shop.transaction)
	}
	assertResponse(t, approve(api, url.Values{"container": {shop.sibling}}), http.StatusNotFound)
}

func TestAPI_007_release_by_stack_needs_no_remaining_container(t *testing.T) {
	env := harness.NewEnv(t)
	shop := freezeShop(t, env)
	env.Docker("rm", "-f", shop.sibling)
	_, api := startAPI(env, "--http-api-update")
	assertResponse(t, approve(api, url.Values{"stack": {"unrelated"}}), http.StatusNotFound)
	response := approve(api, url.Values{"stack": {shop.project}})
	assertResponse(t, response, http.StatusOK)
	body := response.JSON()
	if body["stack"] != shop.project {
		t.Errorf("release body %v", body)
	}
	if transactions, _ := body["transactions"].([]any); len(transactions) != 1 || transactions[0] != shop.transaction {
		t.Errorf("released transactions %v, want [%s]", body["transactions"], shop.transaction)
	}
	assertResponse(t, approve(api, url.Values{"stack": {shop.project}}), http.StatusNotFound)
}

func statusStack(t *testing.T, env *harness.Env, project string) map[string]any {
	t.Helper()
	hosts, _ := env.StatusJSON()["hosts"].([]any)
	for _, host := range hosts {
		stacks, _ := host.(map[string]any)["stacks"].([]any)
		for _, stack := range stacks {
			if stack.(map[string]any)["name"] == project {
				return stack.(map[string]any)
			}
		}
	}
	t.Fatalf("no stack %s in the status document", project)
	return nil
}

func TestSTS_002_frozen_lists_the_transaction_and_its_containers(t *testing.T) {
	env := harness.NewEnv(t)
	shop := freezeShop(t, env)
	frozen, _ := statusStack(t, env, shop.project)["frozen"].([]any)
	if len(frozen) != 1 {
		t.Fatalf("frozen %v, want one entry", frozen)
	}
	entry := frozen[0].(map[string]any)
	if entry["transaction"] != shop.transaction || entry["snapshot"] == "" || entry["snapshot"] == nil {
		t.Errorf("frozen entry %v", entry)
	}
	containers, _ := entry["containers"].([]any)
	if len(containers) != 1 || containers[0] != shop.sibling {
		t.Errorf("frozen containers %v, want [%s]", containers, shop.sibling)
	}
	_, api := startAPI(env, "--http-api-update")
	assertResponse(t, approve(api, url.Values{"stack": {shop.project}}), http.StatusOK)
	if frozen, _ := statusStack(t, env, shop.project)["frozen"].([]any); len(frozen) != 0 {
		t.Errorf("frozen after release %v", frozen)
	}
}

func TestSTS_002_last_transaction_reports_state_result_and_reason(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	untouched := env.RunContainer("b", fixture(env), harness.ContainerOptions{})
	for _, name := range []string{a, untouched} {
		if last := serviceStatus(t, env, name)["last_transaction"]; last != nil {
			t.Errorf("%s has last_transaction %v before any transaction", name, last)
		}
	}
	app.registry.PushSpec("app", "1", harness.ImageSpec{Verify: true, Version: "2", Env: map[string]string{"EXIT_AFTER": "1"}})
	result := armedOnce(env, "--verify-stable", "3s")
	entry := assertState(t, result, a, "rolled_back", "verify_failed")
	last, _ := serviceStatus(t, env, a)["last_transaction"].(map[string]any)
	if last["id"] != entry["txn"] || last["state"] != "rolled_back" || last["result"] != "rolled_back" || last["reason"] != "verify_failed" || last["time"] == nil {
		t.Errorf("last_transaction %v", last)
	}
	assertState(t, armedOnce(env, "--verify-stable", "3s"), a, "held", "previous_attempt_failed")
	if again, _ := serviceStatus(t, env, a)["last_transaction"].(map[string]any); again["id"] != entry["txn"] {
		t.Errorf("last_transaction changed without a transaction: %v", again)
	}
}

func serviceStatus(t *testing.T, env *harness.Env, container string) map[string]any {
	t.Helper()
	hosts, _ := env.StatusJSON()["hosts"].([]any)
	for _, host := range hosts {
		stacks, _ := host.(map[string]any)["stacks"].([]any)
		for _, stack := range stacks {
			services, _ := stack.(map[string]any)["services"].([]any)
			for _, service := range services {
				if service.(map[string]any)["container"] == container {
					return service.(map[string]any)
				}
			}
		}
	}
	t.Fatalf("no service %s in the status document", container)
	return nil
}

func TestSTS_004_view_shows_hold_and_frozen_state(t *testing.T) {
	env := harness.NewEnv(t)
	shop := freezeShop(t, env)
	shop.publishSibling("2")
	assertState(t, shop.update(t), shop.sibling, "held", "previous_attempt_failed")
	stack := statusStack(t, env, shop.project)
	if frozen, _ := stack["frozen"].([]any); len(frozen) == 0 {
		t.Errorf("stack is not shown as frozen: %v", stack)
	}
	hold, _ := serviceStatus(t, env, shop.sibling)["hold"].(map[string]any)
	if hold["reason"] != "previous_attempt_failed" {
		t.Errorf("hold %v", hold)
	}
}

func TestNTF_012_rollback_failure_is_sent_at_once_whatever_the_settings(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	freezeShop(t, env, "--no-startup-message", "--notification-url", webhook.URL(), "--notifications-level", "error", "--notifications-delay", "60")
	received := webhook.Received()
	if len(received) == 0 {
		t.Fatal("no notification received")
	}
	first := received[0]
	if !strings.Contains(first.Message, "txn-") || !strings.Contains(first.Message, "rollback_failed") {
		t.Errorf("first notification does not describe the rollback failure:\n%s", first.Message)
	}
	if len(received) > 1 && received[1].Received.Sub(first.Received) < 30*time.Second {
		t.Errorf("the failure notification waited for the delay: %s then %s", first.Received, received[1].Received)
	}
}

func TestPOL_004_same_tag_update_keeps_the_reference(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	v1 := registry.Push("app", "1.2.0", "1")
	a := env.RunContainer("a", v1, harness.ContainerOptions{})
	registry.Push("app", "1.2.0", "2")
	registry.Push("app", "1.3.0", "3")
	assertUpdated(t, armedOnce(env), a)
	if image := strings.TrimSpace(env.Docker("inspect", "--format", "{{.Config.Image}}", a)); image != v1 {
		t.Errorf("replacement reference %s, want %s", image, v1)
	}
}

func followedImage(t *testing.T, env *harness.Env, label string, args ...string) (string, string) {
	t.Helper()
	registry := env.Registry()
	v1 := registry.Push("app", "1.2.0", "1")
	options := harness.ContainerOptions{}
	if label != "" {
		options.Labels = map[string]string{"parachute.tag-follow": label}
	}
	a := env.RunContainer("a", v1, options)
	for index, tag := range []string{"1.2.3", "1.3.0", "2.0.0", "1.2.4-rc1"} {
		registry.Push("app", tag, string(rune('2'+index)))
	}
	assertUpdated(t, armedOnce(env, args...), a)
	return strings.TrimSpace(env.Docker("inspect", "--format", "{{.Config.Image}}", a)), a
}

func TestPOL_005_patch_follows_the_highest_patch(t *testing.T) {
	env := harness.NewEnv(t)
	image, a := followedImage(t, env, "patch")
	if !strings.HasSuffix(image, "/app:1.2.3") {
		t.Errorf("replacement uses %s", image)
	}
	previous := env.Docker("inspect", "--format", "{{index .Config.Labels \"parachute.previous-image\"}}", a)
	if !strings.Contains(previous, "app:1.2.0") {
		t.Errorf("previous image %s", previous)
	}
}

func TestPOL_005_minor_follows_the_highest_minor(t *testing.T) {
	env := harness.NewEnv(t)
	image, _ := followedImage(t, env, "", "--tag-follow", "minor")
	if !strings.HasSuffix(image, "/app:1.3.0") {
		t.Errorf("replacement uses %s", image)
	}
}

func TestPOL_005_non_version_tag_only_gets_same_tag_updates(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	latest := registry.Push("app", "latest", "1")
	a := env.RunContainer("a", latest, harness.ContainerOptions{Labels: map[string]string{"parachute.tag-follow": "minor"}})
	registry.Push("app", "1.3.0", "3")
	registry.Push("app", "latest", "2")
	assertUpdated(t, armedOnce(env), a)
	if image := strings.TrimSpace(env.Docker("inspect", "--format", "{{.Config.Image}}", a)); image != latest {
		t.Errorf("replacement reference %s, want %s", image, latest)
	}
}
