package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

const sleeper = "sleep 3600"

func gatedFailure(t *testing.T, env *harness.Env, name string) (container, signalFile string, app updatable) {
	t.Helper()
	signalDir := filepath.Join(env.Dir, "signal")
	if err := os.MkdirAll(signalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	app = newUpdatable(env, "app")
	container = env.RunContainer(name, app.ref, harness.ContainerOptions{Extra: []string{"-v", signalDir + ":" + signalDir}, Labels: verifyFailLabels(signalDir)})
	verifyFailUpdate(env, "app")
	return container, filepath.Join(signalDir, "go"), app
}

func waitRenamedOriginal(env *harness.Env, container string) string {
	env.T.Helper()
	var id string
	env.Eventually(2*time.Minute, "renamed original", func() bool {
		id = strings.TrimSpace(env.Docker("ps", "-aq", "--filter", "name="+container+"-parachute-"))
		return id != ""
	})
	return id
}

func transactionObjects(env *harness.Env) string {
	env.T.Helper()
	return strings.TrimSpace(env.Docker("ps", "-aq", "--filter", "label=parachute.transaction"))
}

func dataVolumeApp(env *harness.Env, volume string) (string, string) {
	env.T.Helper()
	registry := env.Registry()
	v1 := registry.PushSpec("app", "1", harness.ImageSpec{Base: "busybox:1.37", Version: "1", Cmd: []string{"sh", "-c", sleeper}})
	a := env.RunContainer("a", v1, harness.ContainerOptions{Extra: []string{"-v", volume + ":/data"}})
	env.Docker("exec", a, "sh", "-c", "echo original > /data/version")
	registry.PushSpec("app", "1", harness.ImageSpec{
		Base: "busybox:1.37", Version: "2",
		Labels: map[string]string{"parachute.verify.cmd": "exit 1", "parachute.verify.timeout": "5s"},
		Cmd:    []string{"sh", "-c", "echo migrated > /data/version; " + sleeper},
	})
	return a, volume
}

func TestVER_008_rollback_restores_originals_and_removes_replacements(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	id, image := env.ContainerState(a)
	env.Registry().PushSpec("app", "1", harness.ImageSpec{Verify: true, Version: "2", Env: map[string]string{"EXIT_AFTER": "1"}})
	result := armedOnce(env, "--verify-stable", "3s")
	assertState(t, result, a, "rolled_back", "verify_failed")
	assertUnchanged(env, a, id, image)
	if running := strings.TrimSpace(env.Docker("inspect", "--format", "{{.State.Running}}", a)); running != "true" {
		t.Fatalf("original not running: %s", running)
	}
	if left := transactionObjects(env); left != "" {
		t.Fatalf("replacement left behind: %s", left)
	}
	var phases []string
	for _, log := range result.Logs() {
		if phase := log.Get("phase"); phase != "" {
			phases = append(phases, phase)
		}
	}
	if joined := strings.Join(phases, ","); !strings.HasSuffix(joined, "verify,rollback") {
		t.Fatalf("phases %s", joined)
	}
}

func TestVER_009_data_restored_when_replacement_started(t *testing.T) {
	env := harness.NewEnv(t)
	a, _ := dataVolumeApp(env, env.ID+"-data")
	result := armedOnce(env, "--verify-stable", "1s")
	entry := assertState(t, result, a, "rolled_back", "verify_failed")
	assertReasonDetail(t, entry, "data restored: yes")
	if got := strings.TrimSpace(env.Docker("exec", a, "cat", "/data/version")); got != "original" {
		t.Fatalf("data not restored: %q", got)
	}
}

func TestVER_010_missing_original_is_rollback_failed(t *testing.T) {
	env := harness.NewEnv(t)
	a, signalFile, _ := gatedFailure(t, env, "a")
	done := runArmedAsync(env, "--backup", "none")
	original := waitRenamedOriginal(env, a)
	env.Docker("rm", "-f", original)
	if err := os.WriteFile(signalFile, []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := <-done
	assertState(t, result, a, "rollback_failed", "restore_failed")
	assertExit(t, result, 3)
}

func TestVER_011_report_names_transaction_check_output_and_data(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	a, _ := dataVolumeApp(env, env.ID+"-data")
	result := armedOnce(env, "--verify-stable", "1s", "--no-startup-message", "--notification-url", webhook.URL())
	entry := assertState(t, result, a, "rolled_back", "verify_failed")
	if txn, _ := entry["txn"].(string); !strings.HasPrefix(txn, "txn-") {
		t.Fatalf("missing transaction id: %v", entry)
	}
	assertReasonDetail(t, entry, "parachute.verify.cmd", "data restored: yes")
	assertContains(t, webhook.WaitFor(time.Minute, 1)[0].Message, "verify_failed", "data restored: yes")
}

func TestVER_011_verify_timeout_names_deadline_and_label(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Labels: map[string]string{"parachute.verify.timeout": "5s"}})
	env.Registry().PushSpec("app", "1", harness.ImageSpec{Verify: true, Version: "2", Healthcheck: []string{"CMD", "/fixture", "exit", "1"}, HealthcheckInterval: "1s"})
	result := armedOnce(env, "--no-startup-message", "--notification-url", webhook.URL())
	entry := assertState(t, result, a, "rolled_back", "verify_timeout")
	assertReasonDetail(t, entry, "5s", "parachute.verify.timeout")
}

func TestUPD_009_commit_removes_renamed_original_and_reports_updated(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	result := armedOnce(env)
	entry := assertState(t, result, a, "updated", "")
	if left := strings.TrimSpace(env.Docker("ps", "-aq", "--filter", "name="+a+"-parachute-")); left != "" {
		t.Fatalf("renamed original left: %s", left)
	}
	if txn, _ := entry["txn"].(string); !strings.HasPrefix(txn, "txn-") {
		t.Fatalf("missing transaction id: %v", entry)
	}
}

func TestUPD_010_cleanup_removes_previous_image_after_commit_only(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	_, previous := env.ContainerState(a)
	app.publishNewer()
	assertState(t, armedOnce(env, "--cleanup"), a, "updated", "")
	if strings.Contains(env.ImageList(), previous) {
		t.Fatalf("previous image %s still present", previous)
	}
}

func TestUPD_010_cleanup_keeps_previous_image_after_rollback(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	id, previous := env.ContainerState(a)
	env.Registry().PushSpec("app", "1", harness.ImageSpec{Verify: true, Version: "2", Env: map[string]string{"EXIT_AFTER": "1"}})
	assertState(t, armedOnce(env, "--cleanup", "--verify-stable", "3s"), a, "rolled_back", "verify_failed")
	assertUnchanged(env, a, id, previous)
}

func TestUPD_014_snapshot_failure_keeps_container(t *testing.T) {
	env := harness.NewEnv(t)
	store := env.TmpfsMount(filepath.Join(env.Dir, "snapstore"))
	ref := pushV2(env, "app", "1", harness.ImageSpec{Version: "1"})
	a := env.RunContainer("a", ref, harness.ContainerOptions{Extra: []string{"-v", env.ID + "-data:/data"}, Labels: map[string]string{"parachute.backup.dump": "head -c 3221225472 /dev/zero"}})
	pushV2(env, "app", "1", harness.ImageSpec{Version: "2"})
	id, image := env.ContainerState(a)
	result := runArmed(env, "--backup-volume", store)
	assertState(t, result, a, "failed", "snapshot_failed")
	assertUnchanged(env, a, id, image)
	if running := strings.TrimSpace(env.Docker("inspect", "--format", "{{.State.Running}}", a)); running != "true" {
		t.Fatalf("container not running: %s", running)
	}
}

func TestUPD_015_failed_stack_does_not_block_unrelated_stack(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	refA := registry.Push("appa", "1", "1")
	refB := registry.Push("appb", "1", "1")
	a := env.RunContainer("a", refA, harness.ContainerOptions{})
	b := env.RunContainer("b", refB, harness.ContainerOptions{})
	registry.PushSpec("appa", "1", harness.ImageSpec{Verify: true, Version: "2", Env: map[string]string{"EXIT_AFTER": "1"}})
	registry.Push("appb", "1", "2")
	result := armedOnce(env, "--verify-stable", "3s")
	assertState(t, result, a, "rolled_back", "verify_failed")
	assertState(t, result, b, "updated", "")
}

func TestUPD_016_verify_phase_interruption_rolls_back_at_next_start(t *testing.T) {
	env := harness.NewEnv(t)
	a, _, _ := gatedFailure(t, env, "a")
	id, image := env.ContainerState(a)
	process := env.Parachute("--run-once", "--armed", "--protect-databases=false", "--delay-days", "0", "--backup", "none",
		"--verify-stable", "1s", "--helper-image", env.ParachuteImage(), "--porcelain", "v1").Start()
	waitRenamedOriginal(env, a)
	process.Signal(syscall.SIGKILL)
	process.Wait(30 * time.Second)
	result := armedOnce(env, "--backup", "none")
	assertState(t, result, a, "rolled_back", "interrupted")
	assertUnchanged(env, a, id, image)
	if left := transactionObjects(env); left != "" {
		t.Fatalf("replacement left behind: %s", left)
	}
}

func TestUPD_016_snapshot_phase_interruption_restarts_containers(t *testing.T) {
	env := harness.NewEnv(t)
	volume := env.ID + "-data"
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Extra: []string{"-v", volume + ":/data"}})
	id, image := env.ContainerState(a)
	env.Docker("run", "--rm", "--entrypoint", "sh", "-v", volume+":/data", env.FixtureV2Image(), "-c", "head -c 104857600 /dev/zero > /data/fill")
	app.publishNewer()
	process := env.Parachute("--run-once", "--armed", "--protect-databases=false", "--delay-days", "0", "--backup", "stop",
		"--helper-image", env.ParachuteImage(), "--porcelain", "v1").Start()
	env.Eventually(2*time.Minute, "snapshot helper", func() bool {
		return strings.TrimSpace(env.Docker("ps", "-q", "--filter", "label=parachute.helper=true", "--filter", "label=parachute.transaction")) != ""
	})
	process.Signal(syscall.SIGKILL)
	process.Wait(30 * time.Second)
	result := armedOnce(env)
	assertState(t, result, a, "failed", "interrupted")
	assertUnchanged(env, a, id, image)
	if running := strings.TrimSpace(env.Docker("inspect", "--format", "{{.State.Running}}", a)); running != "true" {
		t.Fatalf("container not running: %s", running)
	}
}

func TestUPD_017_unrelated_objects_survive_commit_and_rollback(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	refW := registry.Push("web", "1", "1")
	refA := registry.Push("api", "1", "1")
	web := env.RunContainer("web", refW, harness.ContainerOptions{})
	api := env.RunContainer("api", refA, harness.ContainerOptions{})
	lookalike := env.CreateContainer("web-parachute-abc123", refW, harness.ContainerOptions{})
	fake := env.CreateContainer("fakehelper", refW, harness.ContainerOptions{Labels: map[string]string{
		"parachute.helper": "true", "parachute.transaction": "txn-20260101T000000Z-abc123",
	}})
	volume := env.Name("unused")
	env.Docker("volume", "create", volume)
	registry.Push("web", "1", "2")
	registry.PushSpec("api", "1", harness.ImageSpec{Verify: true, Version: "2", Env: map[string]string{"EXIT_AFTER": "1"}})
	result := armedOnce(env, "--cleanup", "--remove-volumes", "--verify-stable", "3s")
	assertState(t, result, web, "updated", "")
	assertState(t, result, api, "rolled_back", "verify_failed")
	for _, name := range []string{lookalike, fake} {
		env.Docker("inspect", name)
	}
	env.Docker("volume", "inspect", volume)
}

func TestUPD_019_transaction_timeout_must_be_positive(t *testing.T) {
	env := harness.NewEnv(t)
	for _, value := range []string{"0", "-1m"} {
		result := env.Parachute("--run-once", "--transaction-timeout="+value).Run(runTimeout)
		assertExit(t, result, 2)
		assertContains(t, result.Stderr, "transaction-timeout")
	}
}

func TestUPD_019_ceiling_rolls_back_with_transaction_timeout(t *testing.T) {
	env := harness.NewEnv(t)
	a, _, _ := gatedFailure(t, env, "a")
	id, image := env.ContainerState(a)
	result := armedOnce(env, "--transaction-timeout", "20s", "--verify-stable", "1s")
	assertState(t, result, a, "rolled_back", "transaction_timeout")
	assertUnchanged(env, a, id, image)
}

func TestUPD_019_clock_pauses_while_helper_copies(t *testing.T) {
	env := harness.NewEnv(t)
	volume := env.ID + "-data"
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Extra: []string{"-v", volume + ":/data"}})
	env.Docker("run", "--rm", "--entrypoint", "sh", "-v", volume+":/data", env.FixtureV2Image(), "-c", "head -c 209715200 /dev/zero > /data/fill")
	app.publishNewer()
	done := runArmedAsync(env, "--backup", "live", "--transaction-timeout", "1m")
	var helper string
	env.Eventually(2*time.Minute, "snapshot helper", func() bool {
		helper = strings.TrimSpace(env.Docker("ps", "-q", "--filter", "label=parachute.helper=true", "--filter", "label=parachute.transaction"))
		return helper != ""
	})
	if _, err := env.DockerMayFail("pause", helper); err != nil {
		t.Skip("snapshot helper already exited")
	}
	time.Sleep(90 * time.Second)
	env.Docker("unpause", helper)
	assertState(t, <-done, a, "updated", "")
}
