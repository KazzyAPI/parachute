package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func lockFileExclusive(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func TestRST_001_restore_verify_exits_zero(t *testing.T) {
	env := harness.NewEnv(t)
	store := filepath.Join(env.Dir, "backup")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newUpdatable(env, "app")
	env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	runArmed(env, "--backup-volume", store)
	result := env.Parachute("restore-verify", "--stack", env.Name("a"), "--backup-volume", store, "--helper-image", env.ParachuteImage()).Run(runTimeout)
	assertExit(t, result, 0)
}

func TestRST_001_lock_held_exits_two(t *testing.T) {
	env := harness.NewEnv(t)
	lock := filepath.Join(env.StateDir, "restore-verify.lock")
	file, err := os.OpenFile(lock, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if err := lockFileExclusive(file); err != nil {
		t.Fatal(err)
	}
	result := env.Parachute("restore-verify", "--helper-image", env.ParachuteImage()).Run(runTimeout)
	assertExit(t, result, 2)
	if !strings.Contains(result.Stderr, "restore-verify.lock") {
		t.Fatalf("stderr %q", result.Stderr)
	}
}

func TestRST_002_throwaway_container_shape(t *testing.T) {
	env := harness.NewEnv(t)
	store := filepath.Join(env.Dir, "backup")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newUpdatable(env, "app")
	env.RunContainer("a", app.ref, harness.ContainerOptions{Labels: map[string]string{"parachute.verify.stable": "3s"}})
	app.publishNewer()
	runArmed(env, "--backup-volume", store)
	result := env.Parachute("restore-verify", "--stack", env.Name("a"), "--backup-volume", store, "--helper-image", env.ParachuteImage()).Run(runTimeout)
	assertExit(t, result, 0)
	ids := strings.Fields(env.Docker("ps", "-aq", "--filter", "label=parachute.restore-verify"))
	if len(ids) > 0 {
		t.Fatalf("leftover throwaways: %v", ids)
	}
}

func TestRST_003_corrupt_archive_fails(t *testing.T) {
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
	path := filepath.Join(store, snap, file)
	env.HostShell("truncate -s 1 " + path)
	result := env.Parachute("restore-verify", "--stack", env.Name("a"), "--backup-volume", store, "--helper-image", env.ParachuteImage()).Run(runTimeout)
	assertRestoreVerifyFail(t, env, result, "snapshot_invalid")
}

func TestRST_004_live_container_unchanged(t *testing.T) {
	env := harness.NewEnv(t)
	store := filepath.Join(env.Dir, "backup")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	id, image := env.ContainerState(a)
	app.publishNewer()
	runArmed(env, "--backup-volume", store)
	result := env.Parachute("restore-verify", "--stack", env.Name("a"), "--backup-volume", store, "--helper-image", env.ParachuteImage()).Run(runTimeout)
	assertExit(t, result, 0)
	assertUnchanged(env, a, id, image)
}

func TestRST_005_cleanup_unlabeled_parachute_rv_names(t *testing.T) {
	env := harness.NewEnv(t)
	keepContainer := env.Docker("run", "-d", "--name", "parachute-rv-abcdef-keep", env.FixtureV2Image(), "sleep", "3600")
	keepVolume := env.ID + "-keepvol"
	env.Docker("volume", "create", "--name", "parachute-rv-abcdef-"+keepVolume)
	keepNetwork := "parachute-rv-abcdef-keepnet"
	env.Docker("network", "create", keepNetwork)
	store := filepath.Join(env.Dir, "backup")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	app := newUpdatable(env, "app")
	env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	runArmed(env, "--backup-volume", store)
	_ = env.Parachute("restore-verify", "--stack", env.Name("a"), "--backup-volume", store, "--helper-image", env.ParachuteImage()).Run(runTimeout)
	if running := env.Docker("inspect", "--format", "{{.State.Running}}", keepContainer); running != "true" {
		t.Fatal("keep container removed")
	}
	if env.Docker("volume", "inspect", "parachute-rv-abcdef-"+keepVolume) == "" {
		t.Fatal("keep volume removed")
	}
	if env.Docker("network", "inspect", keepNetwork) == "" {
		t.Fatal("keep network removed")
	}
}
