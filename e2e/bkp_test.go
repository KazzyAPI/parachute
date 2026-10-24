package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestBKP_001_snapshot_failure_leaves_container_unchanged(t *testing.T) {
	env := harness.NewEnv(t)
	store := env.TmpfsMount(filepath.Join(env.Dir, "snapstore"))
	ref := pushV2(env, "app", "1", harness.ImageSpec{Version: "1", Labels: map[string]string{"parachute.backup.dump": "head -c 3221225472 /dev/zero"}})
	a := env.RunContainer("a", ref, harness.ContainerOptions{Extra: []string{"-v", env.ID + "-data:/data", "--mount", "type=volume,source=" + env.ID + "-data,target=/data"}})
	env.Docker("run", "--rm", "-v", env.ID+"-data:/data", env.FixtureV2Image(), "sh", "-c", "head -c 10485760 /dev/zero > /data/fill")
	pushV2(env, "app", "2", harness.ImageSpec{Version: "2", Labels: map[string]string{"parachute.backup.dump": "head -c 3221225472 /dev/zero"}})
	id, image := env.ContainerState(a)
	result := runArmed(env, "--backup-volume", store)
	assertState(t, result, a, "failed", "snapshot_failed")
	assertUnchanged(env, a, id, image)
}

func TestBKP_001_backup_none_skips_snapshot_data(t *testing.T) {
	env := harness.NewEnv(t)
	ref := pushV2(env, "app", "1", harness.ImageSpec{Version: "1", Labels: map[string]string{"parachute.backup": "none", "parachute.verify.cmd": "exit 1"}})
	a := env.RunContainer("a", ref, harness.ContainerOptions{})
	pushV2(env, "app", "2", harness.ImageSpec{Version: "2", Labels: map[string]string{"parachute.backup": "none", "parachute.verify.cmd": "exit 1"}})
	result := runArmed(env)
	assertState(t, result, a, "rolled_back", "verify_failed")
	if label := snapshotLabel(env, a); label != "none" {
		t.Fatalf("snapshot label %q, want none", label)
	}
}

func TestBKP_002_manifest_lists_writable_mounts_only(t *testing.T) {
	t.Skip("pending full mount manifest assertions")
}

func TestBKP_002_exclude_label_omits_mount(t *testing.T) {
	t.Skip("pending exclude label assertions")
}

func TestBKP_003_stop_mode_stops_before_helper(t *testing.T) {
	t.Skip("pending event ordering")
}

func TestBKP_003_live_mode_copies_before_stop(t *testing.T) {
	t.Skip("pending live mode events")
}

func TestBKP_003_none_mode_rollback_reports_no_data(t *testing.T) {
	t.Skip("pending rollback report detail")
}

func TestBKP_004_postgres_dump_in_snapshot(t *testing.T) {
	t.Skip("pending postgres dump e2e")
}

func TestBKP_004_mysql_password_not_in_events(t *testing.T) {
	t.Skip("pending mysql redaction e2e")
}

func TestBKP_004_custom_dump_failure(t *testing.T) {
	env := harness.NewEnv(t)
	ref := pushV2(env, "app", "1", harness.ImageSpec{Version: "1", Labels: map[string]string{"parachute.backup.dump": "exit 1"}})
	a := env.RunContainer("a", ref, harness.ContainerOptions{})
	pushV2(env, "app", "2", harness.ImageSpec{Version: "2", Labels: map[string]string{"parachute.backup.dump": "exit 1"}})
	id, image := env.ContainerState(a)
	result := runArmed(env)
	assertState(t, result, a, "failed", "dump_failed")
	assertUnchanged(env, a, id, image)
}

func TestBKP_004_dump_stderr_redacted(t *testing.T) {
	t.Skip("pending dump redaction")
}

func TestBKP_004_dump_off_skips_file(t *testing.T) {
	t.Skip("pending dump off")
}

func TestBKP_005_default_volume_contains_snapshot(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	result := runArmed(env)
	assertState(t, result, a, "updated", "")
	snap := snapshotLabel(env, a)
	if snap == "" || snap == "none" {
		t.Fatalf("expected snapshot label, got %q", snap)
	}
	path := backupVolumePath(env, "parachute-snapshots")
	if _, err := os.Stat(filepath.Join(path, snap)); err != nil {
		t.Fatalf("snapshot dir missing: %v", err)
	}
}

func TestBKP_005_host_backup_volume_manifest(t *testing.T) {
	env := harness.NewEnv(t)
	store := filepath.Join(env.Dir, "backup")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	result := runArmed(env, "--backup-volume", store)
	assertState(t, result, a, "updated", "")
	snap := snapshotLabel(env, a)
	if !strings.Contains(env.ReadHostPath(filepath.Join(store, snap, "manifest.json")), snap) {
		t.Fatalf("manifest missing or wrong id")
	}
}

func TestBKP_006_manifest_matches_archives(t *testing.T) {
	t.Skip("pending manifest archive checksums")
}

func TestBKP_007_truncated_archive_invalid_on_rollback(t *testing.T) {
	t.Skip("pending rollback invalid snapshot")
}

func TestBKP_007_manifest_path_traversal_rejected(t *testing.T) {
	t.Skip("pending path traversal")
}

func TestBKP_007_symlink_archive_invalid_on_restore_verify(t *testing.T) {
	t.Skip("pending symlink archive")
}

func TestBKP_008_retention_after_commits(t *testing.T) {
	t.Skip("pending retention")
}

func TestBKP_008_retention_after_rollbacks(t *testing.T) {
	t.Skip("pending retention rollbacks")
}

func TestBKP_008_retention_leaves_unrelated_dirs(t *testing.T) {
	t.Skip("pending retention unrelated")
}

func TestBKP_008_retention_ignores_untracked_snapshot_dirs(t *testing.T) {
	t.Skip("pending retention untracked")
}

func TestBKP_008_retention_does_not_follow_symlinks(t *testing.T) {
	t.Skip("pending retention symlinks")
}

func TestBKP_009_partial_snapshot_removed(t *testing.T) {
	t.Skip("pending partial cleanup")
}

func TestBKP_010_no_transaction_helpers_after_cycle(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	runArmed(env)
	helpers := env.Docker("ps", "-aq", "--filter", "label=parachute.helper=true", "--filter", "label=parachute.transaction")
	if strings.TrimSpace(helpers) != "" {
		t.Fatalf("helpers remain: %s", helpers)
	}
	_ = a
}

func TestBKP_010_helper_hardening(t *testing.T) {
	t.Skip("pending helper inspect assertions")
}

func TestBKP_010_unrelated_helpers_not_removed(t *testing.T) {
	t.Skip("pending helper cleanup scope")
}

func TestBKP_010_recover_removes_stale_helpers(t *testing.T) {
	t.Skip("pending recovery helpers")
}

func TestBKP_010_host_armed_requires_helper_image(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once", "--armed").Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, "helper-image")
}

func TestBKP_010_restore_verify_requires_helper_image(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("restore-verify").Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, "helper-image")
}

func TestBKP_010_unarmed_run_once_ok_without_helper_image(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	result := runOnce(env)
	assertExit(t, result, 0)
	assertContains(t, result.Stderr, "")
	if strings.Contains(result.Stderr, "helper-image") {
		t.Fatalf("unexpected helper-image message")
	}
}

func TestBKP_010_status_ok_without_helper_image(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("status").Run(runTimeout)
	assertExit(t, result, 0)
	if strings.Contains(result.Stdout+result.Stderr, "helper-image") {
		t.Fatalf("unexpected helper-image message")
	}
}

func TestBKP_011_insufficient_space_refuses_update(t *testing.T) {
	env := harness.NewEnv(t)
	store := env.TmpfsMount(filepath.Join(env.Dir, "snapstore"))
	volume := env.ID + "-big"
	env.Docker("volume", "create", volume)
	a := env.RunContainer("a", pushV2(env, "app", "1", harness.ImageSpec{Version: "1"}), harness.ContainerOptions{
		Extra: []string{"-v", volume + ":/data"},
	})
	env.Docker("run", "--rm", "-v", volume+":/data", env.FixtureV2Image(), "sh", "-c", "head -c 1610612736 /dev/zero > /data/fill")
	pushV2(env, "app", "2", harness.ImageSpec{Version: "2"})
	id, image := env.ContainerState(a)
	result := runArmed(env, "--backup-volume", store)
	assertState(t, result, a, "failed", "insufficient_space")
	assertUnchanged(env, a, id, image)
}

func TestBKP_011_small_volume_proceeds(t *testing.T) {
	env := harness.NewEnv(t)
	store := env.TmpfsMount(filepath.Join(env.Dir, "snapstore"))
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Extra: []string{"-v", env.ID + "-small:/data"}})
	env.Docker("run", "--rm", "-v", env.ID+"-small:/data", env.FixtureV2Image(), "sh", "-c", "head -c 104857600 /dev/zero > /data/fill")
	app.publishNewer()
	result := runArmed(env, "--backup-volume", store)
	assertState(t, result, a, "updated", "")
	_ = a
}

func TestBKP_011_backup_none_skips_space_check(t *testing.T) {
	env := harness.NewEnv(t)
	store := env.TmpfsMount(filepath.Join(env.Dir, "snapstore"))
	ref := pushV2(env, "app", "1", harness.ImageSpec{Version: "1", Labels: map[string]string{"parachute.backup": "none"}})
	a := env.RunContainer("a", ref, harness.ContainerOptions{})
	pushV2(env, "app", "2", harness.ImageSpec{Version: "2", Labels: map[string]string{"parachute.backup": "none"}})
	result := runArmed(env, "--backup-volume", store)
	assertState(t, result, a, "updated", "")
	_ = a
}

func TestBKP_012_system_mounts_skipped(t *testing.T) {
	t.Skip("pending system mount manifest")
}

func TestBKP_012_resolved_symlink_skipped(t *testing.T) {
	t.Skip("pending symlink skip")
}

func TestBKP_012_backup_store_mount_skipped(t *testing.T) {
	t.Skip("pending store skip")
}

func TestBKP_012_local_volume_device_skipped(t *testing.T) {
	t.Skip("pending local volume skip")
}

func TestBKP_012_rollback_reports_system_mount(t *testing.T) {
	t.Skip("pending rollback system mount report")
}

func TestBKP_013_archive_escape_rejected(t *testing.T) {
	t.Skip("pending archive escape")
}

func TestBKP_013_symlink_outside_target_rejected(t *testing.T) {
	t.Skip("pending symlink escape")
}

func TestBKP_013_manifest_source_not_used_as_target(t *testing.T) {
	t.Skip("pending restore target")
}
