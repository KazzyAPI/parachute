package e2e

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func ownWithSibling(t *testing.T, env *harness.Env) (own string, sibling string, project string) {
	t.Helper()
	ref := officialRef(env)
	project = env.ID + "-ops"
	webApp := newUpdatable(env, "web")
	sibling = env.RunContainer("web", webApp.ref, harness.ContainerOptions{
		Labels: map[string]string{"com.docker.compose.project": project, "com.docker.compose.service": "web"},
	})
	_, own = runOwnContainer(env, ref, nil, map[string]string{
		"com.docker.compose.project": project, "com.docker.compose.service": "parachute",
	}, func() {})
	return own, sibling, project
}

func injectSelfUpdateFreeze(t *testing.T, env *harness.Env, own, txn string) {
	t.Helper()
	assertExit(t, env.Parachute("--run-once").Run(runTimeout), 0)
	editState(t, env, func(document map[string]any) {
		document["freezes"] = []any{map[string]any{
			"transaction": txn,
			"snapshot":    "none",
			"keys":        []any{map[string]any{"host": "local", "container": own}},
		}}
		document["transactions"] = []any{map[string]any{
			"id": txn, "host": "local", "stacks": []any{env.ID + "-ops"}, "state": "rollback_failed",
			"self_update": true, "own_container": own, "result": "rollback_failed", "reason": "interrupted",
			"time": time.Now().UTC().Format(time.RFC3339),
		}}
	})
}

func TestAPI_007_release_own_container_freeze_without_held_update(t *testing.T) {
	env := harness.NewEnv(t)
	own, _, _ := ownWithSibling(t, env)
	injectSelfUpdateFreeze(t, env, own, "txn-20260101T000001Z-aaaa01")
	process, api := startAPI(env, "--http-api-update")
	defer stopDaemon(process)
	response := approve(api, url.Values{"container": {own}})
	assertResponse(t, response, http.StatusOK)
	body := response.JSON()
	if body["container"] != own {
		t.Errorf("release body %v", body)
	}
	transactions, _ := body["transactions"].([]any)
	if len(transactions) != 1 || transactions[0] != "txn-20260101T000001Z-aaaa01" {
		t.Errorf("released transactions %v", body["transactions"])
	}
	assertResponse(t, approve(api, url.Values{"container": {own}}), http.StatusNotFound)
}

func TestSTS_002_self_update_freeze_lists_only_own_container(t *testing.T) {
	env := harness.NewEnv(t)
	own, sibling, project := ownWithSibling(t, env)
	injectSelfUpdateFreeze(t, env, own, "txn-20260101T000002Z-aaaa02")
	stack := statusStack(t, env, project)
	frozen, _ := stack["frozen"].([]any)
	if len(frozen) != 1 {
		t.Fatalf("frozen %v, want one entry", frozen)
	}
	entry := frozen[0].(map[string]any)
	if entry["transaction"] != "txn-20260101T000002Z-aaaa02" {
		t.Errorf("frozen entry %v", entry)
	}
	containers, _ := entry["containers"].([]any)
	if len(containers) != 1 || containers[0] != own {
		t.Errorf("frozen containers %v, want [%s]; sibling %s must not appear", containers, own, sibling)
	}
}

func TestPOL_010_rollback_failed_self_update_does_not_freeze_project(t *testing.T) {
	env := harness.NewEnv(t)
	own, sibling, _ := ownWithSibling(t, env)
	injectSelfUpdateFreeze(t, env, own, "txn-20260101T000003Z-aaaa03")
	env.Registry().Push("web", "1", "2")
	result := runArmed(env, "--backup", "none")
	entry := result.Entry(sibling)
	if entry == nil {
		t.Fatal("no report entry for sibling")
	}
	if entry["state"] == "held" && entry["reason"] == "previous_attempt_failed" {
		t.Errorf("sibling frozen with project self-update: %v", entry)
	}
}

func TestCFG_010_superseded_instance_recorded_in_state(t *testing.T) {
	env := harness.NewEnv(t)
	ref := officialRef(env)
	own := startOwnWithStateMount(env, ref)
	oldID, _ := env.ContainerState(own)
	txn := "txn-20260101T000100Z-bbbb01"
	digest := imageRepoDigest(t, env, ref)
	writeHandover(t, env, map[string]any{
		"format": "parachute.handover/v1", "transaction": txn, "container": oldID, "digest": digest,
	})
	editState(t, env, func(document map[string]any) {
		document["transactions"] = []any{map[string]any{
			"id": txn, "host": "local", "stacks": []any{env.Scope()}, "state": "handover",
			"self_update": true, "own_container": own, "time": time.Now().UTC().Format(time.RFC3339),
			"originals": []any{map[string]any{"id": oldID, "running": true, "updated": true}},
		}}
	})
	webhook := env.Webhook()
	successor := env.StartParachuteContainer("successor", harness.ContainerRun{
		Image:  ref,
		Mounts: []string{env.StateDir + ":/var/lib/parachute", env.ConfigDir + ":/root/.docker"},
		Args: []string{
			"--state-dir", "/var/lib/parachute", "--notification-url", webhook.ContainerURL(), "--notifications-level", "error",
			"--interval", "31536000", "--scope", env.Scope(),
		},
		Labels: map[string]string{"parachute.transaction": txn},
	})
	notifications := webhook.WaitForContainer(30*time.Second, 1, successor)
	if !mentions(notifications, txn) {
		t.Fatalf("notifications %v", notifications)
	}
	env.OpenStateDir()
	document := env.StateDocument()
	instances, _ := document["superseded_instances"].([]any)
	if len(instances) != 1 || instances[0] != oldID {
		t.Fatalf("superseded_instances %v", document["superseded_instances"])
	}
}

func TestNTF_012_interrupted_handover_sends_error_notification(t *testing.T) {
	env := harness.NewEnv(t)
	ref := officialRef(env)
	own := startOwnWithStateMount(env, ref)
	oldID, _ := env.ContainerState(own)
	txn := "txn-20260101T000200Z-bbbb02"
	digest := imageRepoDigest(t, env, ref)
	writeHandover(t, env, map[string]any{
		"format": "parachute.handover/v1", "transaction": txn, "container": oldID, "digest": digest,
	})
	editState(t, env, func(document map[string]any) {
		document["transactions"] = []any{map[string]any{
			"id": txn, "host": "local", "stacks": []any{env.Scope()}, "state": "handover",
			"self_update": true, "own_container": own, "time": time.Now().UTC().Format(time.RFC3339),
			"originals": []any{map[string]any{"id": oldID, "running": true, "updated": true}},
		}}
	})
	webhook := env.Webhook()
	successor := env.StartParachuteContainer("handover-successor", harness.ContainerRun{
		Image:  ref,
		Mounts: []string{env.StateDir + ":/var/lib/parachute", env.ConfigDir + ":/root/.docker"},
		Args: []string{
			"--state-dir", "/var/lib/parachute", "--notification-url", webhook.ContainerURL(), "--notifications-level", "error",
			"--interval", "31536000", "--scope", env.Scope(),
		},
		Labels: map[string]string{"parachute.transaction": txn},
	})
	notifications := webhook.WaitForContainer(30*time.Second, 1, successor)
	if !mentions(notifications, txn) {
		t.Fatalf("notifications %v", notifications)
	}
}

func mentions(notifications []harness.Notification, transaction string) bool {
	for _, notification := range notifications {
		if strings.Contains(notification.Message, transaction) {
			return true
		}
	}
	return false
}
