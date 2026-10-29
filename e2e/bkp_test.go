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

func TestBKP_001_snapshot_failure_leaves_container_unchanged(t *testing.T) {
	env := harness.NewEnv(t)
	store := env.TmpfsMount(filepath.Join(env.Dir, "snapstore"))
	ref := pushV2(env, "app", "1", harness.ImageSpec{Version: "1", Labels: map[string]string{"parachute.backup.dump": "head -c 3221225472 /dev/zero"}})
	a := env.RunContainer("a", ref, harness.ContainerOptions{Extra: []string{"-v", env.ID + "-data:/data"}})
	env.Docker("run", "--rm", "--entrypoint", "sh", "-v", env.ID+"-data:/data", env.FixtureV2Image(), "-c", "head -c 10485760 /dev/zero > /data/fill")
	pushV2(env, "app", "1", harness.ImageSpec{Version: "2", Labels: map[string]string{"parachute.backup.dump": "head -c 3221225472 /dev/zero"}})
	id, image := env.ContainerState(a)
	result := runArmed(env, "--backup-volume", store)
	assertState(t, result, a, "failed", "snapshot_failed")
	assertUnchanged(env, a, id, image)
}

func TestBKP_001_backup_none_skips_snapshot_data(t *testing.T) {
	env := harness.NewEnv(t)
	ref := pushV2(env, "app", "1", harness.ImageSpec{Version: "1", Labels: map[string]string{"parachute.backup": "none", "parachute.verify.cmd": "exit 1"}})
	a := env.RunContainer("a", ref, harness.ContainerOptions{})
	pushV2(env, "app", "1", harness.ImageSpec{Version: "2", Labels: map[string]string{"parachute.backup": "none", "parachute.verify.cmd": "exit 1"}})
	result := runArmed(env)
	assertState(t, result, a, "rolled_back", "verify_failed")
	if label := snapshotLabel(env, a); label != "none" {
		t.Fatalf("snapshot label %q, want none", label)
	}
}

func TestBKP_002_manifest_lists_writable_mounts_only(t *testing.T) {
	env := harness.NewEnv(t)
	store := filepath.Join(env.Dir, "backup")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(env.Dir, "conf")
	ro := filepath.Join(env.Dir, "ro")
	sock := filepath.Join(env.Dir, "docker.sock")
	for _, path := range []string{conf, ro} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(sock, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	volume := env.ID + "-data"
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Extra: []string{
		"-v", volume + ":/data",
		"-v", conf + ":/conf",
		"-v", ro + ":/etc/ro:ro",
		"-v", sock + ":/var/run/docker.sock",
	}})
	app.publishNewer()
	runArmed(env, "--backup-volume", store)
	snap := snapshotLabel(env, a)
	manifest := readManifest(env, store, snap)
	mounts := manifestStrings(manifest, "mounts", "destination")
	for _, want := range []string{"/data", "/conf"} {
		if !strings.Contains(strings.Join(mounts, " "), want) {
			t.Fatalf("mounts %v missing %s", mounts, want)
		}
	}
	for _, omit := range []string{"/etc/ro", "/var/run/docker.sock"} {
		for _, dest := range mounts {
			if dest == omit {
				t.Fatalf("mounts should not include %s", omit)
			}
		}
	}
}

func TestBKP_002_exclude_label_omits_mount(t *testing.T) {
	env := harness.NewEnv(t)
	store := filepath.Join(env.Dir, "backup")
	media := filepath.Join(env.Dir, "media")
	if err := os.MkdirAll(media, 0o755); err != nil {
		t.Fatal(err)
	}
	ref := pushV2(env, "app", "1", harness.ImageSpec{Version: "1", Labels: map[string]string{"parachute.backup.exclude": "/media"}})
	a := env.RunContainer("a", ref, harness.ContainerOptions{Extra: []string{"-v", media + ":/media"}})
	pushV2(env, "app", "1", harness.ImageSpec{Version: "2", Labels: map[string]string{"parachute.backup.exclude": "/media"}})
	runArmed(env, "--backup-volume", store)
	snap := snapshotLabel(env, a)
	manifest := readManifest(env, store, snap)
	for _, dest := range manifestStrings(manifest, "mounts", "destination") {
		if dest == "/media" {
			t.Fatalf("manifest lists excluded mount /media")
		}
	}
}

func TestBKP_003_stop_mode_stops_before_helper(t *testing.T) {
	env := harness.NewEnv(t)
	rec := env.StartRawEventRecorder()
	app := newUpdatable(env, "app")
	name := env.Name("a")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Extra: []string{"-v", env.ID + "-data:/data"}})
	app.publishNewer()
	runArmed(env)
	rec.AssertOrder("die "+name, "start")
	_ = a
}

func TestBKP_003_live_mode_copies_before_stop(t *testing.T) {
	env := harness.NewEnv(t)
	rec := env.StartRawEventRecorder()
	ref := pushV2(env, "app", "1", harness.ImageSpec{Version: "1", Labels: map[string]string{"parachute.backup": "live"}})
	name := env.Name("a")
	a := env.RunContainer("a", ref, harness.ContainerOptions{Extra: []string{"-v", env.ID + "-data:/data"}})
	pushV2(env, "app", "1", harness.ImageSpec{Version: "2", Labels: map[string]string{"parachute.backup": "live"}})
	runArmed(env)
	events := rec.Events()
	dieIndex, startIndex := -1, -1
	for index, line := range events {
		if strings.Contains(line, "die") && strings.Contains(line, name) {
			dieIndex = index
		}
		if strings.Contains(line, "start") && !strings.Contains(line, name) && startIndex < 0 {
			startIndex = index
		}
	}
	if startIndex < 0 || dieIndex < 0 || dieIndex < startIndex {
		t.Fatalf("expected helper start before container die in live mode, events=%v", events)
	}
	_ = a
}

func TestBKP_003_none_mode_rollback_reports_no_data(t *testing.T) {
	env := harness.NewEnv(t)
	ref := pushV2(env, "app", "1", harness.ImageSpec{Version: "1", Labels: map[string]string{"parachute.backup": "none", "parachute.verify.cmd": "exit 1"}})
	a := env.RunContainer("a", ref, harness.ContainerOptions{})
	pushV2(env, "app", "1", harness.ImageSpec{Version: "2", Labels: map[string]string{"parachute.backup": "none", "parachute.verify.cmd": "exit 1"}})
	result := runArmed(env)
	entry := assertState(t, result, a, "rolled_back", "verify_failed")
	assertReasonDetail(t, entry, "data not restored: backup=none")
}

func TestBKP_004_postgres_dump_in_snapshot(t *testing.T) {
	env := harness.NewEnv(t)
	store := filepath.Join(env.Dir, "backup")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	registry := env.Registry()
	ref := registry.PushSpec("library/postgres", "16", harness.ImageSpec{Version: "1", Base: "postgres:16-alpine"})
	volume := env.ID + "-pgdata"
	db := env.RunContainer("db", ref, harness.ContainerOptions{Extra: []string{
		"-e", "POSTGRES_PASSWORD=postgres",
		"-v", volume + ":/var/lib/postgresql/data",
	}})
	postgresReady(env, db)
	env.Docker("exec", db, "psql", "-U", "postgres", "-c", "CREATE TABLE rows(id int); INSERT INTO rows VALUES (42);")
	registry.PushSpec("library/postgres", "16", harness.ImageSpec{Version: "2", Base: "postgres:16-alpine"})
	result := runArmed(env, "--backup-volume", store)
	assertState(t, result, db, "updated", "")
	snap := snapshotLabel(env, db)
	manifest := readManifest(env, store, snap)
	dumps := manifestStrings(manifest, "dumps", "file")
	if len(dumps) == 0 {
		t.Fatal("expected postgres dump in snapshot")
	}
	dumpPath := filepath.Join(store, snap, dumps[0])
	restoreVol := env.ID + "-restore"
	env.Docker("volume", "create", restoreVol)
	restored := env.RunContainer("pgrestore", registry.PushSpec("library/postgres", "16", harness.ImageSpec{Version: "1", Base: "postgres:16-alpine"}), harness.ContainerOptions{Extra: []string{
		"-e", "POSTGRES_PASSWORD=postgres",
		"-v", restoreVol + ":/var/lib/postgresql/data",
	}})
	postgresReady(env, restored)
	env.Docker("run", "--rm", "--network", "container:"+restored, "-v", dumpPath+":/dump.sql:ro",
		"postgres:16-alpine", "psql", "-h", "127.0.0.1", "-U", "postgres", "-f", "/dump.sql")
	out := env.Docker("exec", restored, "psql", "-U", "postgres", "-tAc", "SELECT id FROM rows;")
	if !strings.Contains(strings.TrimSpace(out), "42") {
		t.Fatalf("dump did not restore row, got %q", out)
	}
}

func TestBKP_004_mysql_password_not_in_events(t *testing.T) {
	env := harness.NewEnv(t)
	rec := env.StartRawEventRecorder()
	ref := pushRealImage(env, "library/mysql", "8", "mysql:8-oracle")
	volume := env.ID + "-mysqldata"
	db := env.RunContainer("db", ref, harness.ContainerOptions{Extra: []string{
		"-e", "MYSQL_ROOT_PASSWORD=s3cr3t-pw",
		"-v", volume + ":/var/lib/mysql",
	}})
	env.Eventually(2*time.Minute, "mysql ready", func() bool {
		_, err := env.DockerMayFail("exec", db, "mysqladmin", "ping", "-uroot", "-ps3cr3t-pw")
		return err == nil
	})
	env.Registry().PushSpec("library/mysql", "8", harness.ImageSpec{Version: "2", Base: "mysql:8-oracle"})
	result := runArmed(env, "--log-level", "debug")
	assertState(t, result, db, "updated", "")
	for _, line := range append(rec.Events(), result.Stderr) {
		if strings.Contains(line, "s3cr3t-pw") {
			t.Fatalf("secret leaked in %q", line)
		}
	}
}

func TestBKP_004_custom_dump_failure(t *testing.T) {
	env := harness.NewEnv(t)
	ref := pushV2(env, "app", "1", harness.ImageSpec{Version: "1", Labels: map[string]string{"parachute.backup.dump": "exit 1"}})
	a := env.RunContainer("a", ref, harness.ContainerOptions{})
	pushV2(env, "app", "1", harness.ImageSpec{Version: "2", Labels: map[string]string{"parachute.backup.dump": "exit 1"}})
	id, image := env.ContainerState(a)
	result := runArmed(env)
	assertState(t, result, a, "failed", "dump_failed")
	assertUnchanged(env, a, id, image)
}

func TestBKP_004_dump_stderr_redacted(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	ref := pushV2(env, "app", "1", harness.ImageSpec{Version: "1", Labels: map[string]string{
		"parachute.database": "true",
	}, Env: map[string]string{"MYSQL_ROOT_PASSWORD": "s3cr3t-pw"}})
	a := env.RunContainer("a", ref, harness.ContainerOptions{Extra: []string{"-e", "MYSQL_ROOT_PASSWORD=s3cr3t-pw"}})
	pushV2(env, "app", "1", harness.ImageSpec{Version: "2", Labels: map[string]string{
		"parachute.database":    "true",
		"parachute.backup.dump": `echo "$MYSQL_ROOT_PASSWORD" >&2; exit 1`,
	}, Env: map[string]string{"MYSQL_ROOT_PASSWORD": "s3cr3t-pw"}})
	result := notify(env, webhook, "--log-level", "debug")
	assertState(t, result, a, "failed", "dump_failed")
	assertNoSecrets(t, "stderr", result.Stderr, "s3cr3t-pw")
	assertNoSecrets(t, "stdout", result.Stdout, "s3cr3t-pw")
	assertContains(t, result.Stderr, "[REDACTED]")
	assertNotificationsHaveNoSecrets(t, webhook, "s3cr3t-pw")
}

func TestBKP_004_dump_off_skips_file(t *testing.T) {
	env := harness.NewEnv(t)
	store := filepath.Join(env.Dir, "backup")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	registry := env.Registry()
	ref := registry.PushSpec("library/postgres", "16", harness.ImageSpec{Version: "1", Base: "postgres:16-alpine", Labels: map[string]string{"parachute.backup.dump": "off"}})
	db := env.RunContainer("db", ref, harness.ContainerOptions{Extra: []string{"-e", "POSTGRES_PASSWORD=postgres", "-v", env.ID + "-pg:/var/lib/postgresql/data"}})
	postgresReady(env, db)
	registry.PushSpec("library/postgres", "16", harness.ImageSpec{Version: "2", Base: "postgres:16-alpine", Labels: map[string]string{"parachute.backup.dump": "off"}})
	runArmed(env, "--backup-volume", store)
	snap := snapshotLabel(env, db)
	manifest := readManifest(env, store, snap)
	if dumps := manifestStrings(manifest, "dumps", "file"); len(dumps) != 0 {
		t.Fatalf("expected no dumps, got %v", dumps)
	}
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
	env := harness.NewEnv(t)
	store := filepath.Join(env.Dir, "backup")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Extra: []string{"-v", env.ID + "-data:/data"}})
	app.publishNewer()
	runArmed(env, "--backup-volume", store)
	snap := snapshotLabel(env, a)
	manifest := readManifest(env, store, snap)
	if manifest["transaction"] == nil {
		t.Fatal("manifest missing transaction")
	}
	assertManifestArchivesMatch(t, env, store, snap)
}

func TestBKP_007_truncated_archive_invalid_on_rollback(t *testing.T) {
	env := harness.NewEnv(t)
	store := filepath.Join(env.Dir, "backup")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	signalDir := filepath.Join(env.Dir, "tamper")
	if err := os.MkdirAll(signalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	signalFile := filepath.Join(signalDir, "go")
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Extra: []string{
		"-v", env.ID + "-data:/data",
		"-v", signalDir + ":" + signalDir,
	}})
	verifyFailUpdate(env, "app", &app, signalDir)
	done := runArmedAsync(env, "--backup-volume", store)
	waitSnapshotDuringVerify(env, "a", signalFile, func(snapID string) {
		manifest := readManifest(env, store, snapID)
		record := firstMount(env, manifest)
		file, _ := record["file"].(string)
		path := filepath.Join(store, snapID, file)
		env.WriteHostPath(path, "truncated")
	})
	result := <-done
	id, image := env.ContainerState(a)
	assertState(t, result, a, "rollback_failed", "snapshot_invalid")
	assertUnchanged(env, a, id, image)
}

func TestBKP_007_manifest_path_traversal_rejected(t *testing.T) {
	env := harness.NewEnv(t)
	store := filepath.Join(env.Dir, "backup")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	signalDir := filepath.Join(env.Dir, "tamper")
	if err := os.MkdirAll(signalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	signalFile := filepath.Join(signalDir, "go")
	outside := filepath.Join(env.Dir, "outside.tar.gz")
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Extra: []string{
		"-v", env.ID + "-data:/data",
		"-v", signalDir + ":" + signalDir,
	}})
	verifyFailUpdate(env, "app", &app, signalDir)
	done := runArmedAsync(env, "--backup-volume", store)
	waitSnapshotDuringVerify(env, "a", signalFile, func(snapID string) {
		manifest := readManifest(env, store, snapID)
		record := firstMount(env, manifest)
		file, _ := record["file"].(string)
		data := []byte(env.ReadHostPath(filepath.Join(store, snapID, file)))
		if err := os.WriteFile(outside, data, 0o644); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		record["file"] = "../../outside.tar.gz"
		record["sha256"] = hex.EncodeToString(sum[:])
		record["size"] = float64(len(data))
		encoded, _ := json.Marshal(manifest)
		env.WriteHostPath(filepath.Join(store, snapID, "manifest.json"), string(encoded))
	})
	result := <-done
	assertState(t, result, a, "rollback_failed", "snapshot_invalid")
}

func TestBKP_007_symlink_archive_invalid_on_restore_verify(t *testing.T) {
	env := harness.NewEnv(t)
	store := filepath.Join(env.Dir, "backup")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Extra: []string{"-v", env.ID + "-data:/data"}})
	app.publishNewer()
	runArmed(env, "--backup-volume", store)
	snap := snapshotLabel(env, a)
	manifest := readManifest(env, store, snap)
	record := firstMount(env, manifest)
	file, _ := record["file"].(string)
	archivePath := filepath.Join(store, snap, file)
	real := archivePath + ".real"
	env.HostShell("mv " + archivePath + " " + real + " && ln -s " + real + " " + archivePath)
	result := env.Parachute("restore-verify", "--backup-volume", store, "--helper-image", env.ParachuteImage()).Run(runTimeout)
	assertExit(t, result, 1)
	assertContains(t, result.Stderr, "snapshot_invalid")
	_ = snap
}

func TestBKP_008_retention_after_commits(t *testing.T) {
	env := harness.NewEnv(t)
	store := filepath.Join(env.Dir, "backup")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	stack := env.Name("a")
	for range 4 {
		app.publishNewer()
		runArmed(env, "--backup-volume", store)
	}
	if count := countStackSnapshots(env, store, stack); count != 3 {
		t.Fatalf("retention: got %d snapshots, want 3", count)
	}
	_ = a
}

func TestBKP_008_retention_after_rollbacks(t *testing.T) {
	env := harness.NewEnv(t)
	store := filepath.Join(env.Dir, "backup")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	stack := env.Name("a")
	for range 5 {
		ref := pushV2(env, "app", "1", harness.ImageSpec{Version: "1", Labels: map[string]string{"parachute.verify.cmd": "exit 1"}})
		a := env.RunContainer("a", ref, harness.ContainerOptions{})
		pushV2(env, "app", "1", harness.ImageSpec{Version: "2", Labels: map[string]string{"parachute.verify.cmd": "exit 1"}})
		runArmed(env, "--backup-volume", store)
		env.Docker("rm", "-f", a)
	}
	if count := countStackSnapshots(env, store, stack); count != 3 {
		t.Fatalf("retention after rollbacks: got %d snapshots, want 3", count)
	}
}

func TestBKP_008_retention_leaves_unrelated_dirs(t *testing.T) {
	env := harness.NewEnv(t)
	store := filepath.Join(env.Dir, "backup")
	notes := filepath.Join(store, "notes")
	if err := os.MkdirAll(notes, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newUpdatable(env, "app")
	env.RunContainer("a", app.ref, harness.ContainerOptions{})
	for range 4 {
		app.publishNewer()
		runArmed(env, "--backup-volume", store)
	}
	if _, err := os.Stat(notes); err != nil {
		t.Fatalf("notes dir removed: %v", err)
	}
}

func TestBKP_008_retention_ignores_untracked_snapshot_dirs(t *testing.T) {
	env := harness.NewEnv(t)
	store := filepath.Join(env.Dir, "backup")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newUpdatable(env, "app")
	env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	runArmed(env, "--backup-volume", store)
	snap := firstSnapshotDir(env, store)
	fakeID := "snap-20990101T000000Z-abcdef"
	manifest := readManifest(env, store, snap)
	manifest["id"] = fakeID
	encoded, _ := json.Marshal(manifest)
	fakeDir := filepath.Join(store, fakeID)
	if err := os.MkdirAll(fakeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	env.WriteHostPath(filepath.Join(fakeDir, "manifest.json"), string(encoded))
	for range 3 {
		app.publishNewer()
		runArmed(env, "--backup-volume", store)
	}
	if _, err := os.Stat(fakeDir); err != nil {
		t.Fatalf("untracked snapshot dir removed: %v", err)
	}
}

func TestBKP_008_retention_does_not_follow_symlinks(t *testing.T) {
	env := harness.NewEnv(t)
	store := filepath.Join(env.Dir, "backup")
	keep := filepath.Join(env.Dir, "keep")
	if err := os.MkdirAll(keep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keep, "data"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := newUpdatable(env, "app")
	env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	runArmed(env, "--backup-volume", store)
	oldest := firstSnapshotDir(env, store)
	env.HostShell("rm -rf " + filepath.Join(store, oldest) + " && ln -s " + keep + " " + filepath.Join(store, oldest))
	for range 3 {
		app.publishNewer()
		runArmed(env, "--backup-volume", store)
	}
	if _, err := os.Stat(filepath.Join(keep, "data")); err != nil {
		t.Fatalf("symlink target lost: %v", err)
	}
}

func TestBKP_009_partial_snapshot_removed(t *testing.T) {
	env := harness.NewEnv(t)
	store := env.TmpfsMount(filepath.Join(env.Dir, "snapstore"))
	volume := env.ID + "-data"
	a := env.RunContainer("a", pushV2(env, "app", "1", harness.ImageSpec{Version: "1"}), harness.ContainerOptions{
		Extra: []string{"-v", volume + ":/data"},
	})
	env.Docker("run", "--rm", "--entrypoint", "sh", "-v", volume+":/data", env.FixtureV2Image(), "-c", "head -c 10485760 /dev/zero > /data/fill")
	pushV2(env, "app", "1", harness.ImageSpec{Version: "2"})
	done := runArmedAsync(env, "--backup-volume", store)
	env.Eventually(2*time.Minute, "container stopped for snapshot", func() bool {
		state := env.Docker("inspect", "--format", "{{.State.Running}}", a)
		return state == "false"
	})
	mountpoint := strings.TrimSpace(env.Docker("volume", "inspect", "--format", "{{.Mountpoint}}", volume))
	env.HostShell("chmod 000 " + mountpoint)
	result := <-done
	if before := listSnapshotDirs(env, store); len(before) > 0 {
		t.Fatalf("partial snapshot dir remains: %v", before)
	}
	env.HostShell("chmod 755 " + mountpoint)
	running := env.Docker("inspect", "--format", "{{.State.Running}}", a)
	if running != "true" {
		t.Fatalf("container not running after failed snapshot, got %s", running)
	}
	assertState(t, result, a, "failed", "snapshot_failed")
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
	env := harness.NewEnv(t)
	volume := env.ID + "-big"
	app := newUpdatable(env, "app")
	inspectHelperOnCreate(env, volume, func() {
		env.RunContainer("a", app.ref, harness.ContainerOptions{Extra: []string{"-v", volume + ":/data"}})
		env.Docker("run", "--rm", "--entrypoint", "sh", "-v", volume+":/data", env.FixtureV2Image(), "-c", "head -c 52428800 /dev/zero > /data/fill")
		app.publishNewer()
		runArmed(env)
	})
}

func TestBKP_010_unrelated_helpers_not_removed(t *testing.T) {
	env := harness.NewEnv(t)
	fake := createFakeHelper(env, nil)
	fakeTxn := createFakeHelper(env, map[string]string{"parachute.transaction": "txn-20260101T000000Z-abcdef"})
	app := newUpdatable(env, "app")
	env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	runArmed(env)
	for _, id := range []string{fake, fakeTxn} {
		if running := env.Docker("inspect", "--format", "{{.State.Running}}", id); running != "true" {
			t.Fatalf("helper %s removed", id)
		}
	}
}

func TestBKP_010_recover_removes_stale_helpers(t *testing.T) {
	env := harness.NewEnv(t)
	volume := env.ID + "-big"
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Extra: []string{"-v", volume + ":/data"}})
	killDuringSnapshot(env, volume, &app, a)
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
	env.Docker("run", "--rm", "--entrypoint", "sh", "-v", volume+":/data", env.FixtureV2Image(), "-c", "head -c 1610612736 /dev/zero > /data/fill")
	pushV2(env, "app", "1", harness.ImageSpec{Version: "2"})
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
	env.Docker("run", "--rm", "--entrypoint", "sh", "-v", env.ID+"-small:/data", env.FixtureV2Image(), "-c", "head -c 104857600 /dev/zero > /data/fill")
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
	pushV2(env, "app", "1", harness.ImageSpec{Version: "2", Labels: map[string]string{"parachute.backup": "none"}})
	result := runArmed(env, "--backup-volume", store)
	assertState(t, result, a, "updated", "")
	_ = a
}

func TestBKP_012_system_mounts_skipped(t *testing.T) {
	env := harness.NewEnv(t)
	store := filepath.Join(env.Dir, "backup")
	etc := filepath.Join(env.Dir, "host-etc")
	proc := filepath.Join(env.Dir, "host-proc")
	data := filepath.Join(env.Dir, "appdata")
	for _, path := range []string{etc, proc, data} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	ref := pushV2(env, "app", "1", harness.ImageSpec{Version: "1"})
	a := env.RunContainer("a", ref, harness.ContainerOptions{Extra: []string{
		"-v", etc + ":/host-etc",
		"-v", proc + ":/host-proc",
		"-v", data + ":/data",
	}})
	pushV2(env, "app", "1", harness.ImageSpec{Version: "2"})
	runArmed(env, "--backup-volume", store)
	snap := snapshotLabel(env, a)
	manifest := readManifest(env, store, snap)
	mounts := manifestStrings(manifest, "mounts", "destination")
	skipped := manifestStrings(manifest, "skipped", "destination")
	if !strings.Contains(strings.Join(mounts, " "), "/data") {
		t.Fatalf("expected /data in mounts, got %v", mounts)
	}
	for _, dest := range []string{"/host-etc", "/host-proc"} {
		found := false
		for _, item := range skipped {
			if item == dest {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected %s skipped, got %v", dest, skipped)
		}
	}
}

func TestBKP_012_resolved_symlink_skipped(t *testing.T) {
	env := harness.NewEnv(t)
	store := filepath.Join(env.Dir, "backup")
	link := filepath.Join(env.Dir, "etc-link")
	if err := os.MkdirAll(filepath.Join(env.Dir, "cfg"), 0o755); err != nil {
		t.Fatal(err)
	}
	env.HostShell("ln -sf /etc " + link)
	ref := pushV2(env, "app", "1", harness.ImageSpec{Version: "1"})
	a := env.RunContainer("a", ref, harness.ContainerOptions{Extra: []string{"-v", link + ":/cfg"}})
	pushV2(env, "app", "1", harness.ImageSpec{Version: "2"})
	runArmed(env, "--backup-volume", store)
	snap := snapshotLabel(env, a)
	skipped := manifestStrings(readManifest(env, store, snap), "skipped", "destination")
	if !strings.Contains(strings.Join(skipped, " "), "/cfg") {
		t.Fatalf("/cfg not skipped: %v", skipped)
	}
}

func TestBKP_012_backup_store_mount_skipped(t *testing.T) {
	env := harness.NewEnv(t)
	store := filepath.Join(env.Dir, "backup")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	srv := filepath.Join(env.Dir, "srv")
	if err := os.MkdirAll(filepath.Join(srv, "parachute"), 0o755); err != nil {
		t.Fatal(err)
	}
	ref := pushV2(env, "app", "1", harness.ImageSpec{Version: "1"})
	a := env.RunContainer("a", ref, harness.ContainerOptions{Extra: []string{"-v", srv + ":/srv"}})
	pushV2(env, "app", "1", harness.ImageSpec{Version: "2"})
	runArmed(env, "--backup-volume", store)
	snap := snapshotLabel(env, a)
	skipped := manifestStrings(readManifest(env, store, snap), "skipped", "destination")
	if !strings.Contains(strings.Join(skipped, " "), "/srv") {
		t.Fatalf("/srv not skipped when store under it: %v", skipped)
	}
}

func TestBKP_012_local_volume_device_skipped(t *testing.T) {
	env := harness.NewEnv(t)
	store := filepath.Join(env.Dir, "backup")
	volume := env.ID + "-local"
	env.Docker("volume", "create", "--driver", "local", "--opt", "type=none", "--opt", "o=bind", "--opt", "device=/etc", volume)
	ref := pushV2(env, "app", "1", harness.ImageSpec{Version: "1"})
	a := env.RunContainer("a", ref, harness.ContainerOptions{Extra: []string{"-v", volume + ":/conf"}})
	pushV2(env, "app", "1", harness.ImageSpec{Version: "2"})
	runArmed(env, "--backup-volume", store)
	snap := snapshotLabel(env, a)
	skipped := manifestStrings(readManifest(env, store, snap), "skipped", "destination")
	if !strings.Contains(strings.Join(skipped, " "), "/conf") {
		t.Fatalf("/conf not skipped for local bind device /etc: %v", skipped)
	}
}

func TestBKP_012_rollback_reports_system_mount(t *testing.T) {
	env := harness.NewEnv(t)
	etc := filepath.Join(env.Dir, "host-etc")
	if err := os.MkdirAll(etc, 0o755); err != nil {
		t.Fatal(err)
	}
	ref := pushV2(env, "app", "1", harness.ImageSpec{Version: "1", Labels: map[string]string{"parachute.verify.cmd": "exit 1"}})
	a := env.RunContainer("a", ref, harness.ContainerOptions{Extra: []string{"-v", etc + ":/host-etc"}})
	pushV2(env, "app", "1", harness.ImageSpec{Version: "2", Labels: map[string]string{"parachute.verify.cmd": "exit 1"}})
	result := runArmed(env)
	entry := assertState(t, result, a, "rolled_back", "verify_failed")
	assertReasonDetail(t, entry, "data not restored: system mount /host-etc")
}

func TestBKP_013_archive_escape_rejected(t *testing.T) {
	env := harness.NewEnv(t)
	store := filepath.Join(env.Dir, "backup")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	volume := env.ID + "-data"
	signalDir := filepath.Join(env.Dir, "tamper")
	if err := os.MkdirAll(signalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	signalFile := filepath.Join(signalDir, "go")
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Extra: []string{
		"-v", volume + ":/data",
		"-v", signalDir + ":" + signalDir,
	}})
	verifyFailUpdate(env, "app", &app, signalDir)
	mountpoint := strings.TrimSpace(env.Docker("volume", "inspect", "--format", "{{.Mountpoint}}", volume))
	done := runArmedAsync(env, "--backup-volume", store)
	waitSnapshotDuringVerify(env, "a", signalFile, func(snapID string) {
		manifest := readManifest(env, store, snapID)
		record := firstMount(env, manifest)
		file, _ := record["file"].(string)
		replaceArchive(env, store, snapID, file, "mkdir -p /t && echo bad > /t/x && tar -czf - -C /t --transform 's/x/..\\/escape/' x")
	})
	result := <-done
	assertState(t, result, a, "rollback_failed", "snapshot_invalid")
	if _, err := os.Stat(filepath.Join(filepath.Dir(mountpoint), "escape")); err == nil {
		t.Fatal("escape file created outside volume")
	}
}

func TestBKP_013_symlink_outside_target_rejected(t *testing.T) {
	env := harness.NewEnv(t)
	store := filepath.Join(env.Dir, "backup")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Extra: []string{"-v", env.ID + "-data:/data"}})
	app.publishNewer()
	runArmed(env, "--backup-volume", store)
	snap := snapshotLabel(env, a)
	manifest := readManifest(env, store, snap)
	record := firstMount(env, manifest)
	file, _ := record["file"].(string)
	replaceArchive(env, store, snap, file, "mkdir -p /t && ln -s /etc /t/link && tar -czf - -C /t link")
	before := env.ReadHostPath("/etc/passwd")
	result := env.Parachute("restore-verify", "--backup-volume", store, "--helper-image", env.ParachuteImage()).Run(runTimeout)
	assertExit(t, result, 1)
	assertContains(t, result.Stderr, "snapshot_invalid")
	after := env.ReadHostPath("/etc/passwd")
	if before != after {
		t.Fatal("/etc/passwd changed after failed restore-verify")
	}
	_ = a
}

func TestBKP_013_manifest_source_not_used_as_target(t *testing.T) {
	env := harness.NewEnv(t)
	store := filepath.Join(env.Dir, "backup")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(env.Dir, "victim")
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(victim, "marker"), []byte("victim"), 0o644); err != nil {
		t.Fatal(err)
	}
	volume := env.ID + "-data"
	signalDir := filepath.Join(env.Dir, "tamper")
	if err := os.MkdirAll(signalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	signalFile := filepath.Join(signalDir, "go")
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Extra: []string{
		"-v", volume + ":/data",
		"-v", signalDir + ":" + signalDir,
	}})
	env.Docker("run", "--rm", "--entrypoint", "sh", "-v", volume+":/data", env.FixtureV2Image(), "-c", "echo original > /data/marker")
	verifyFailUpdate(env, "app", &app, signalDir)
	mountpoint := strings.TrimSpace(env.Docker("volume", "inspect", "--format", "{{.Mountpoint}}", volume))
	done := runArmedAsync(env, "--backup-volume", store)
	waitSnapshotDuringVerify(env, "a", signalFile, func(snapID string) {
		manifest := readManifest(env, store, snapID)
		record := firstMount(env, manifest)
		record["source"] = victim
		encoded, _ := json.Marshal(manifest)
		env.WriteHostPath(filepath.Join(store, snapID, "manifest.json"), string(encoded))
		env.WriteHostPath(filepath.Join(mountpoint, "marker"), "replaced")
	})
	result := <-done
	assertState(t, result, a, "rolled_back", "verify_failed")
	if got := env.ReadHostPath(filepath.Join(mountpoint, "marker")); strings.TrimSpace(got) != "original" {
		t.Fatalf("volume marker %q, want original", got)
	}
	if got := env.ReadHostPath(filepath.Join(victim, "marker")); strings.TrimSpace(got) != "victim" {
		t.Fatalf("victim marker changed: %q", got)
	}
}
