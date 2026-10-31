package e2e

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

const selfUpdateTimeout = 3 * time.Minute

func pushRegistryParachute(env *harness.Env, build harness.ParachuteBuild) string {
	env.T.Helper()
	registry := env.Registry()
	ref := registry.Ref("parachute", "1")
	env.BuildParachuteImageSpec(ref, build)
	registry.PushImage(ref)
	return ref
}

func selfUpdateMounts(env *harness.Env) []string {
	return []string{env.StateDir + ":/var/lib/parachute", env.ConfigDir + ":/root/.docker"}
}

func selfUpdateRunArgs(env *harness.Env, extra ...string) []string {
	base := []string{
		"--run-once", "--armed", "--protect-databases=false", "--delay-days", "0",
		"--verify-stable", "1s", "--verify-timeout", "5m", "--helper-image", env.ParachuteImage(),
		"--porcelain", "v1", "--backup", "none", "--scope", env.Scope(),
	}
	return append(base, extra...)
}

func createSelfUpdateOwn(env *harness.Env, ref string, extra []string, labels map[string]string, args ...string) string {
	env.T.Helper()
	run := harness.ContainerRun{
		Args:   selfUpdateRunArgs(env, args...),
		Labels: labels,
		Mounts: selfUpdateMounts(env),
	}
	return env.CreateParachuteContainer("parachute", ref, extra, run)
}

func startSelfUpdateAttachedAsync(env *harness.Env, own string) chan *harness.Result {
	env.T.Helper()
	done := make(chan *harness.Result, 1)
	go func() { done <- env.StartAttached(own, selfUpdateTimeout) }()
	return done
}

func approveSelfUpdate(t *testing.T, env *harness.Env, own string) string {
	t.Helper()
	first := env.StartAttached(own, selfUpdateTimeout)
	assertExit(t, first, 0)
	digest := assertState(t, first, own, "held", "approval_required")["candidate_digest"].(string)
	env.OpenStateDir()
	ownID, _ := env.ContainerState(own)
	editState(t, env, func(document map[string]any) {
		document["approvals"] = []any{map[string]any{"host": "local", "container": ownID, "digest": digest}}
	})
	return digest
}

func runningParachuteContainers(env *harness.Env) []string {
	env.T.Helper()
	names := strings.Fields(env.Docker("ps", "--filter", "label=parachute-e2e="+env.ID, "--filter", "label=parachute.scope="+env.Scope(), "--format", "{{.Names}}"))
	var running []string
	for _, name := range names {
		if strings.TrimSpace(env.Docker("inspect", "--format", "{{index .Config.Labels \"parachute.helper\"}}", name)) == "true" {
			continue
		}
		entrypoint := env.Docker("inspect", "--format", "{{json .Config.Entrypoint}}", name)
		if strings.Contains(entrypoint, "parachute") {
			running = append(running, name)
		}
	}
	return running
}

func handoverMarker(t *testing.T, env *harness.Env) map[string]any {
	t.Helper()
	env.OpenStateDir()
	contents, err := os.ReadFile(filepath.Join(env.StateDir, "handover.json"))
	if err != nil {
		t.Fatal(err)
	}
	var marker map[string]any
	if err := json.Unmarshal(contents, &marker); err != nil {
		t.Fatal(err)
	}
	return marker
}

func transactionState(env *harness.Env, id string) (string, bool) {
	env.OpenStateDir()
	records, _ := env.StateDocument()["transactions"].([]any)
	for _, record := range records {
		entry := record.(map[string]any)
		if entry["id"] == id {
			state, _ := entry["state"].(string)
			return state, true
		}
	}
	return "", false
}

func waitForHandoverMarker(t *testing.T, env *harness.Env, timeout time.Duration) map[string]any {
	t.Helper()
	env.OpenStateDir()
	env.Eventually(timeout, "handover.json", func() bool {
		_, err := os.Stat(filepath.Join(env.StateDir, "handover.json"))
		return err == nil
	})
	return handoverMarker(t, env)
}

func waitForNoHandoverMarker(t *testing.T, env *harness.Env, timeout time.Duration) {
	t.Helper()
	env.OpenStateDir()
	env.Eventually(timeout, "handover.json removed", func() bool {
		_, err := os.Stat(filepath.Join(env.StateDir, "handover.json"))
		return os.IsNotExist(err)
	})
}

func parachuteSuffix(txn string) string {
	return "-parachute-" + txn[len(txn)-6:]
}

func containerNamedParachuteTxn(env *harness.Env, baseName, txn string) string {
	want := baseName + parachuteSuffix(txn)
	names := strings.Fields(env.Docker("ps", "-a", "--filter", "label=parachute-e2e="+env.ID, "--format", "{{.Names}}"))
	for _, name := range names {
		if name == want {
			return name
		}
	}
	return ""
}

func containerRepoDigest(t *testing.T, env *harness.Env, name string) string {
	t.Helper()
	line := env.Docker("inspect", "--format", "{{index .RepoDigests 0}}", name)
	_, digest, ok := strings.Cut(strings.TrimSpace(line), "@")
	if !ok || digest == "" {
		t.Fatalf("repo digest for %s: %q", name, line)
	}
	return digest
}

func lastTransaction(t *testing.T, env *harness.Env, container string) map[string]any {
	t.Helper()
	document := statusDocument(t, env)
	entry := statusServices(document)[container]
	last, _ := entry["last_transaction"].(map[string]any)
	return last
}

func publishDelayedHealthy(env *harness.Env, ref string) {
	env.BuildParachuteImageSpec(ref, harness.ParachuteBuild{
		Version: "2", Labels: map[string]string{selfLabel: "true"},
		Healthcheck: []string{"CMD", "parachute", "healthcheck"}, HealthcheckStartPeriod: "30s", HealthcheckInterval: "5s",
	})
	env.Registry().PushImage(ref)
}

func publishAlwaysUnhealthy(env *harness.Env, ref string) {
	env.BuildParachuteImageSpec(ref, harness.ParachuteBuild{
		Version: "2", Labels: map[string]string{selfLabel: "true"}, Healthcheck: []string{"CMD", "false"},
	})
	env.Registry().PushImage(ref)
}

func TestUPD_018_other_stack_commits_before_self_update_snapshot(t *testing.T) {
	env := harness.NewEnv(t)
	ref := pushRegistryParachute(env, harness.ParachuteBuild{Version: "1", Labels: map[string]string{selfLabel: "true"}})
	project := env.ID + "-ops"
	webApp := newUpdatable(env, "web")
	sibling := env.RunContainer("web", webApp.ref, harness.ContainerOptions{
		Labels: map[string]string{"com.docker.compose.project": project, "com.docker.compose.service": "web", "parachute.scope": env.Scope()},
	})
	webApp.publishNewer()
	labels := map[string]string{"com.docker.compose.project": project, "com.docker.compose.service": "parachute", "parachute.scope": env.Scope()}
	own := createSelfUpdateOwn(env, ref, []string{"--restart=no"}, labels)
	publishDelayedHealthy(env, ref)
	approved := approveSelfUpdate(t, env, own)
	originalID, _ := env.ContainerState(own)
	done := startSelfUpdateAttachedAsync(env, own)
	marker := waitForHandoverMarker(t, env, selfUpdateTimeout)
	txn := marker["transaction"].(string)
	result := <-done
	logs := result.Logs()
	commitBeforeSnapshot := false
	snapshotSeen := false
	for _, entry := range logs {
		if entry.Get("phase") == "commit" && entry.Get("txn") != txn {
			commitBeforeSnapshot = true
		}
		if entry.Get("phase") == "snapshot" && entry.Get("txn") == txn {
			if !commitBeforeSnapshot {
				t.Fatalf("self-update snapshot before sibling commit")
			}
			snapshotSeen = true
		}
	}
	if !snapshotSeen {
		t.Fatal("no self-update snapshot log line")
	}
	assertExit(t, result, 0)
	assertState(t, result, own, "updated", "")
	if running := runningParachuteContainers(env); len(running) != 1 || running[0] != own {
		t.Fatalf("running Parachute containers %v, want [%s]", running, own)
	}
	if afterID, _ := env.ContainerState(own); afterID == originalID {
		t.Fatalf("own container id unchanged %s", afterID)
	}
	if digest := containerRepoDigest(t, env, own); digest != approved {
		t.Fatalf("running digest %s, approved %s", digest, approved)
	}
	if renamed := containerNamedParachuteTxn(env, own, txn); renamed != "" {
		t.Fatalf("renamed transaction container still exists: %s", renamed)
	}
	waitForNoHandoverMarker(t, env, time.Minute)
	last := lastTransaction(t, env, own)
	if last["state"] != "committed" {
		t.Fatalf("last_transaction %v", last)
	}
	_ = sibling
}

func TestUPD_018_failing_health_check_rolls_back_original_identity(t *testing.T) {
	env := harness.NewEnv(t)
	ref := pushRegistryParachute(env, harness.ParachuteBuild{Version: "1", Labels: map[string]string{selfLabel: "true"}})
	own := createSelfUpdateOwn(env, ref, []string{"--restart=no"}, scoped(env))
	publishAlwaysUnhealthy(env, ref)
	approveSelfUpdate(t, env, own)
	originalID, _ := env.ContainerState(own)
	result := env.StartAttached(own, selfUpdateTimeout)
	assertExit(t, result, 0)
	assertState(t, result, own, "rolled_back", "verify_failed")
	if warnings := findLogs(result.Logs(), func(entry harness.Log) bool {
		return entry.Level() == "warn" && entry.Get("container") == own && entry.Get("state") == "rolled_back"
	}); len(warnings) == 0 {
		t.Fatal("no warn rolled_back entry")
	}
	if afterID, _ := env.ContainerState(own); afterID != originalID {
		t.Fatalf("container id changed %s -> %s", originalID, afterID)
	}
	if name := strings.TrimPrefix(env.Docker("inspect", "--format", "{{.Name}}", own), "/"); name != own {
		t.Fatalf("container name changed %q", name)
	}
	waitForNoHandoverMarker(t, env, time.Minute)
	second := env.StartAttached(own, selfUpdateTimeout)
	assertExit(t, second, 0)
	assertState(t, second, own, "held", "previous_attempt_failed")
}

func TestUPD_018_handover_phase_state_marker_and_waiting_log(t *testing.T) {
	env := harness.NewEnv(t)
	ref := pushRegistryParachute(env, harness.ParachuteBuild{Version: "1", Labels: map[string]string{selfLabel: "true"}})
	own := createSelfUpdateOwn(env, ref, []string{"--restart=no"}, scoped(env))
	publishDelayedHealthy(env, ref)
	approveSelfUpdate(t, env, own)
	done := startSelfUpdateAttachedAsync(env, own)
	marker := waitForHandoverMarker(t, env, selfUpdateTimeout)
	txn := marker["transaction"].(string)
	if state, ok := transactionState(env, txn); !ok || state != "handover" {
		t.Fatalf("transaction state %q ok=%v", state, ok)
	}
	if marker["container"] == "" || marker["transaction"] != txn {
		t.Fatalf("marker %v", marker)
	}
	env.Eventually(selfUpdateTimeout, "waiting for handover log", func() bool {
		logs, _ := env.DockerMayFail("logs", own)
		return strings.Contains(logs, "waiting for handover") && strings.Contains(logs, txn)
	})
	logs, _ := env.DockerMayFail("logs", own)
	for _, line := range strings.Split(logs, "\n") {
		if !strings.Contains(line, "waiting for handover") {
			continue
		}
		var fields map[string]any
		if json.Unmarshal([]byte(line), &fields) == nil && fields["phase"] != nil && fields["phase"] != "" {
			t.Fatalf("waiting log carries phase: %s", line)
		}
	}
	result := <-done
	assertExit(t, result, 0)
	assertState(t, result, own, "updated", "")
	waitForNoHandoverMarker(t, env, time.Minute)
}

func TestUPD_018_kill_old_during_verification_interrupts_with_freeze(t *testing.T) {
	env := harness.NewEnv(t)
	ref := pushRegistryParachute(env, harness.ParachuteBuild{Version: "1", Labels: map[string]string{selfLabel: "true"}})
	webhook := env.Webhook()
	port := env.FreePort()
	portText := strconv.Itoa(port)
	own := createSelfUpdateOwn(env, ref, []string{"--restart=no", "-p", "127.0.0.1:" + portText + ":" + portText}, scoped(env),
		"--notification-url", webhook.ContainerURL(), "--notifications-level", "error",
		"--http-api-port", portText, "--http-api-token", apiToken)
	publishDelayedHealthy(env, ref)
	approveSelfUpdate(t, env, own)
	done := startSelfUpdateAttachedAsync(env, own)
	marker := waitForHandoverMarker(t, env, selfUpdateTimeout)
	oldID := marker["container"].(string)
	env.Docker("kill", oldID)
	waitForNoHandoverMarker(t, env, 60*time.Second)
	<-done
	if running := runningParachuteContainers(env); len(running) != 1 {
		t.Fatalf("running parachute containers %v", running)
	}
	last := lastTransaction(t, env, own)
	if last["state"] != "rollback_failed" || last["reason"] != "interrupted" {
		t.Fatalf("last_transaction %v", last)
	}
	if notifications := webhook.Matching(last["id"].(string)); len(notifications) == 0 {
		t.Fatalf("no error notification for %v", last["id"])
	}
	if _, err := env.DockerMayFail("inspect", oldID); err != nil {
		t.Fatalf("old container removed: %v", err)
	}
	stack := statusStack(t, env, env.Scope())
	frozen, _ := stack["frozen"].([]any)
	if len(frozen) != 1 {
		t.Fatalf("frozen %v", frozen)
	}
	entry := frozen[0].(map[string]any)
	containers, _ := entry["containers"].([]any)
	if len(containers) != 1 || containers[0] != own {
		t.Fatalf("frozen containers %v", containers)
	}
}

func TestUPD_018_approve_releases_freeze_after_successor_runs_digest(t *testing.T) {
	env := harness.NewEnv(t)
	ref := pushRegistryParachute(env, harness.ParachuteBuild{Version: "1", Labels: map[string]string{selfLabel: "true"}})
	own := createSelfUpdateOwn(env, ref, []string{"--restart=no"}, scoped(env))
	publishDelayedHealthy(env, ref)
	approveSelfUpdate(t, env, own)
	done := startSelfUpdateAttachedAsync(env, own)
	marker := waitForHandoverMarker(t, env, selfUpdateTimeout)
	oldID := marker["container"].(string)
	txn := marker["transaction"].(string)
	env.Docker("kill", oldID)
	waitForNoHandoverMarker(t, env, 60*time.Second)
	<-done
	process, api := startAPI(env, "--http-api-update")
	defer stopDaemon(process)
	response := approve(api, url.Values{"container": {own}})
	assertResponse(t, response, http.StatusOK)
	body := response.JSON()
	if body["transactions"].([]any)[0] != txn {
		t.Fatalf("approve body %v", body)
	}
	stack := statusStack(t, env, env.Scope())
	if frozen, _ := stack["frozen"].([]any); len(frozen) != 0 {
		t.Fatalf("frozen %v", frozen)
	}
	if currentID, _ := env.ContainerState(own); currentID == oldID {
		t.Fatalf("successor id still old %s", oldID)
	}
}

func TestUPD_018_committed_report_survives_hanging_webhook_and_old_kill(t *testing.T) {
	env := harness.NewEnv(t)
	ref := pushRegistryParachute(env, harness.ParachuteBuild{Version: "1", Labels: map[string]string{selfLabel: "true"}})
	hook := env.HangingWebhook()
	own := createSelfUpdateOwn(env, ref, []string{"--restart=no"}, scoped(env),
		"--notification-url", hook.ContainerURL(), "--notifications-level", "error")
	publishDelayedHealthy(env, ref)
	approved := approveSelfUpdate(t, env, own)
	done := startSelfUpdateAttachedAsync(env, own)
	marker := waitForHandoverMarker(t, env, selfUpdateTimeout)
	oldID := marker["container"].(string)
	txn := marker["transaction"].(string)
	env.Eventually(selfUpdateTimeout, "committed in state", func() bool {
		state, ok := transactionState(env, txn)
		return ok && state == "committed"
	})
	hook.WaitForRequest(2 * time.Minute)
	env.Docker("kill", oldID)
	waitForNoHandoverMarker(t, env, 60*time.Second)
	hook.Release()
	<-done
	if running := runningParachuteContainers(env); len(running) != 1 {
		t.Fatalf("running parachute containers %v", running)
	}
	if renamed := containerNamedParachuteTxn(env, own, txn); renamed != "" {
		t.Fatalf("renamed container %s still exists", renamed)
	}
	last := lastTransaction(t, env, own)
	if last["state"] != "committed" || last["result"] != "updated" {
		t.Fatalf("last_transaction %v", last)
	}
	if digest := containerRepoDigest(t, env, own); digest != approved {
		t.Fatalf("digest %s approved %s", digest, approved)
	}
	stack := statusStack(t, env, env.Scope())
	if frozen, _ := stack["frozen"].([]any); len(frozen) != 0 {
		t.Fatalf("frozen %v", frozen)
	}
	for _, notification := range hook.Received() {
		if strings.Contains(strings.ToLower(notification.Message), "error") {
			t.Fatalf("error notification %q", notification.Message)
		}
	}
}

func TestUPD_018_pause_new_restart_old_rolls_back_interrupted(t *testing.T) {
	env := harness.NewEnv(t)
	ref := pushRegistryParachute(env, harness.ParachuteBuild{Version: "1", Labels: map[string]string{selfLabel: "true"}})
	own := createSelfUpdateOwn(env, ref, []string{"--restart=no"}, scoped(env))
	publishDelayedHealthy(env, ref)
	approveSelfUpdate(t, env, own)
	done := startSelfUpdateAttachedAsync(env, own)
	marker := waitForHandoverMarker(t, env, selfUpdateTimeout)
	oldID := marker["container"].(string)
	txn := marker["transaction"].(string)
	replacementName := own
	env.Eventually(selfUpdateTimeout, "replacement running", func() bool {
		status := strings.TrimSpace(env.Docker("inspect", "--format", "{{.State.Status}}", replacementName))
		return status == "running"
	})
	env.Docker("pause", replacementName)
	env.Docker("restart", oldID)
	waitForNoHandoverMarker(t, env, 60*time.Second)
	<-done
	if afterID, _ := env.ContainerState(own); afterID != oldID {
		t.Fatalf("own id %s, want old %s", afterID, oldID)
	}
	if running := strings.TrimSpace(env.Docker("inspect", "--format", "{{.State.Running}}", own)); running != "true" {
		t.Fatalf("old container not running")
	}
	if replacement := containerNamedParachuteTxn(env, own, txn); replacement != "" {
		t.Fatalf("replacement still exists as %s", replacement)
	}
	last := lastTransaction(t, env, own)
	if last["state"] != "rolled_back" || last["reason"] != "interrupted" {
		t.Fatalf("last_transaction %v", last)
	}
	stack := statusStack(t, env, env.Scope())
	if frozen, _ := stack["frozen"].([]any); len(frozen) != 0 {
		t.Fatalf("frozen %v", frozen)
	}
}

func TestUPD_018_interrupted_self_update_does_not_freeze_compose_sibling(t *testing.T) {
	env := harness.NewEnv(t)
	ref := pushRegistryParachute(env, harness.ParachuteBuild{Version: "1", Labels: map[string]string{selfLabel: "true"}})
	project := env.ID + "-ops"
	webApp := newUpdatable(env, "web")
	sibling := env.RunContainer("web", webApp.ref, harness.ContainerOptions{
		Labels: map[string]string{"com.docker.compose.project": project, "com.docker.compose.service": "web", "parachute.scope": env.Scope()},
	})
	labels := map[string]string{"com.docker.compose.project": project, "com.docker.compose.service": "parachute", "parachute.scope": env.Scope()}
	own := createSelfUpdateOwn(env, ref, []string{"--restart=no"}, labels)
	publishDelayedHealthy(env, ref)
	approveSelfUpdate(t, env, own)
	done := startSelfUpdateAttachedAsync(env, own)
	marker := waitForHandoverMarker(t, env, selfUpdateTimeout)
	oldID := marker["container"].(string)
	env.Docker("kill", oldID)
	waitForNoHandoverMarker(t, env, 60*time.Second)
	<-done
	webApp.publishNewer()
	result := runArmed(env, "--backup", "none")
	entry := result.Entry(sibling)
	if entry == nil || entry["state"] != "updated" {
		t.Fatalf("sibling update %v", entry)
	}
	env.Docker("start", oldID)
	env.Eventually(60*time.Second, "killed old exits", func() bool {
		code := strings.TrimSpace(env.Docker("inspect", "--format", "{{.State.ExitCode}}", oldID))
		return code == "2"
	})
	if running := runningParachuteContainers(env); len(running) != 1 {
		t.Fatalf("running parachute containers %v", running)
	}
}

func TestUPD_018_restart_old_after_successful_handover_exits_superseded(t *testing.T) {
	env := harness.NewEnv(t)
	ref := pushRegistryParachute(env, harness.ParachuteBuild{Version: "1", Labels: map[string]string{selfLabel: "true"}})
	own := createSelfUpdateOwn(env, ref, []string{"--restart=no"}, scoped(env))
	publishDelayedHealthy(env, ref)
	approveSelfUpdate(t, env, own)
	done := startSelfUpdateAttachedAsync(env, own)
	marker := waitForHandoverMarker(t, env, selfUpdateTimeout)
	oldID := marker["container"].(string)
	rec := env.StartEventRecorder("")
	rec.Wait(selfUpdateTimeout, "health_status: healthy "+own)
	env.Docker("pause", own)
	waitForNoHandoverMarker(t, env, selfUpdateTimeout)
	env.Docker("unpause", own)
	<-done
	env.Docker("stop", oldID)
	result := env.StartAttached(oldID, time.Minute)
	assertExit(t, result, 2)
}
