package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func runArmed(env *harness.Env, args ...string) *harness.Result {
	env.T.Helper()
	base := []string{"--armed", "--protect-databases=false", "--delay-days", "0", "--verify-stable", "1s", "--verify-timeout", "30s", "--helper-image", env.ParachuteImage()}
	return runOnce(env, append(base, args...)...)
}

func pushV2(env *harness.Env, repo, tag string, spec harness.ImageSpec) string {
	env.T.Helper()
	spec.Base = env.FixtureV2Image()
	return env.Registry().PushSpec(repo, tag, spec)
}

func snapshotLabel(env *harness.Env, container string) string {
	env.T.Helper()
	return strings.TrimSpace(env.Docker("inspect", "--format", "{{index .Config.Labels \"parachute.snapshot\"}}", container))
}

func readManifest(env *harness.Env, dir, snapshotID string) map[string]any {
	env.T.Helper()
	raw := env.ReadHostPath(filepath.Join(dir, snapshotID, "manifest.json"))
	var document map[string]any
	if err := json.Unmarshal([]byte(raw), &document); err != nil {
		env.T.Fatal(err)
	}
	return document
}

func backupVolumePath(env *harness.Env, volume string) string {
	env.T.Helper()
	return strings.TrimSpace(env.Docker("volume", "inspect", "--format", "{{.Mountpoint}}", volume))
}

func assertReasonDetail(t *testing.T, entry map[string]any, parts ...string) {
	t.Helper()
	detail, _ := entry["detail"].(string)
	for _, part := range parts {
		if !strings.Contains(detail, part) {
			t.Errorf("detail %q missing %q", detail, part)
		}
	}
}

func manifestStrings(m map[string]any, key, field string) []string {
	raw, _ := m[key].([]any)
	var out []string
	for _, item := range raw {
		record, _ := item.(map[string]any)
		if value, ok := record[field].(string); ok {
			out = append(out, value)
		}
	}
	return out
}

func assertManifestArchivesMatch(t *testing.T, env *harness.Env, storeDir, snapshotID string) {
	t.Helper()
	manifest := readManifest(env, storeDir, snapshotID)
	for _, key := range []string{"mounts", "dumps"} {
		raw, _ := manifest[key].([]any)
		for _, item := range raw {
			record, _ := item.(map[string]any)
			file, _ := record["file"].(string)
			wantSHA, _ := record["sha256"].(string)
			path := filepath.Join(storeDir, snapshotID, file)
			data := []byte(env.ReadHostPath(path))
			sum := sha256.Sum256(data)
			if hex.EncodeToString(sum[:]) != wantSHA {
				t.Fatalf("checksum mismatch for %s", file)
			}
		}
	}
}

func listSnapshotDirs(env *harness.Env, storeDir string) []string {
	env.T.Helper()
	out := env.Docker("run", "--rm", "-v", storeDir+":"+storeDir+":ro", env.FixtureV2Image(), "ls", "-1", storeDir)
	var snaps []string
	for _, name := range strings.Fields(out) {
		if strings.HasPrefix(name, "snap-") {
			snaps = append(snaps, name)
		}
	}
	return snaps
}

func countStackSnapshots(env *harness.Env, storeDir, stack string) int {
	env.T.Helper()
	count := 0
	for _, id := range listSnapshotDirs(env, storeDir) {
		manifest := readManifest(env, storeDir, id)
		if manifest["stack"] == stack {
			count++
		}
	}
	return count
}

func verifyFailUpdate(env *harness.Env, repo string, app *updatable, signalDir string) string {
	env.T.Helper()
	signalFile := filepath.Join(signalDir, "go")
	cmd := fmt.Sprintf("while [ ! -f %s ]; do sleep 0.1; done; exit 1", signalFile)
	ref := pushV2(env, repo, "2", harness.ImageSpec{
		Version: "2",
		Labels:  map[string]string{"parachute.verify.cmd": cmd},
	})
	return ref
}

func waitSnapshotDuringVerify(env *harness.Env, container, signalFile string, tamper func(snapID string)) {
	env.T.Helper()
	name := env.Name(container)
	env.Eventually(2*time.Minute, "snapshot label during verify", func() bool {
		return strings.HasPrefix(snapshotLabel(env, name), "snap-")
	})
	snap := snapshotLabel(env, name)
	tamper(snap)
	if err := os.WriteFile(signalFile, []byte("1"), 0o644); err != nil {
		env.T.Fatal(err)
	}
}

func runArmedAsync(env *harness.Env, args ...string) chan *harness.Result {
	env.T.Helper()
	done := make(chan *harness.Result, 1)
	go func() { done <- runArmed(env, args...) }()
	return done
}

func pushRealImage(env *harness.Env, repo, tag, source string) string {
	env.T.Helper()
	env.Docker("pull", source)
	ref := env.Registry().Ref(repo, tag)
	env.Docker("tag", source, ref)
	env.Docker("push", ref)
	return ref
}

func postgresReady(env *harness.Env, container string) {
	env.T.Helper()
	env.Eventually(2*time.Minute, "postgres ready", func() bool {
		_, err := env.DockerMayFail("exec", container, "pg_isready", "-U", "postgres")
		return err == nil
	})
}

func inspectHelperOnCreate(env *harness.Env, run func()) {
	env.T.Helper()
	var once sync.Once
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
			}
			ids := strings.Fields(env.Docker("ps", "-aq", "--filter", "label=parachute.helper=true"))
			for _, id := range ids {
				once.Do(func() {
					spec := env.Docker("inspect", id)
					if !strings.Contains(spec, `"Privileged": false`) {
						env.T.Error("helper privileged")
					}
					if !strings.Contains(spec, `"ReadonlyRootfs": true`) {
						env.T.Error("helper readonly rootfs")
					}
					if strings.Contains(spec, `"NetworkMode": "host"`) {
						env.T.Error("helper host network")
					}
					close(done)
				})
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
	run()
	close(done)
}

func createFakeHelper(env *harness.Env, labels map[string]string) string {
	env.T.Helper()
	args := []string{"run", "-d", "--label", "parachute-e2e=" + env.ID, "--label", "parachute.helper=true"}
	for key, value := range labels {
		args = append(args, "--label", key+"="+value)
	}
	args = append(args, env.FixtureV2Image(), "sleep", "3600")
	return env.Docker(args...)
}

func replaceArchive(env *harness.Env, storeDir, snapID, file string, shellScript string) {
	env.T.Helper()
	snapDir := filepath.Join(storeDir, snapID)
	env.Docker("run", "--rm", "-v", snapDir+":/snap", env.FixtureV2Image(), "sh", "-c", shellScript+" > /snap/"+file)
	data := []byte(env.ReadHostPath(filepath.Join(snapDir, file)))
	sum := sha256.Sum256(data)
	manifest := readManifest(env, storeDir, snapID)
	raw, _ := manifest["mounts"].([]any)
	record, _ := raw[0].(map[string]any)
	record["sha256"] = hex.EncodeToString(sum[:])
	record["size"] = float64(len(data))
	raw[0] = record
	manifest["mounts"] = raw
	encoded, _ := json.Marshal(manifest)
	env.WriteHostPath(filepath.Join(snapDir, "manifest.json"), string(encoded))
}

func killDuringSnapshot(env *harness.Env, volume string, app *updatable, container string) {
	env.T.Helper()
	env.Docker("run", "--rm", "-v", volume+":/data", env.FixtureV2Image(), "sh", "-c", "head -c 104857600 /dev/zero > /data/fill")
	app.publishNewer()
	process := env.Parachute("--run-once", "--armed", "--protect-databases=false", "--delay-days", "0",
		"--verify-stable", "1s", "--verify-timeout", "30s", "--helper-image", env.ParachuteImage(), "--porcelain", "v1").Start()
	env.Eventually(2*time.Minute, "snapshot helper", func() bool {
		return strings.TrimSpace(env.Docker("ps", "-q", "--filter", "label=parachute.helper=true", "--filter", "label=parachute.transaction")) != ""
	})
	process.Signal(syscall.SIGKILL)
	process.Wait(30 * time.Second)
	runArmed(env)
	helpers := env.Docker("ps", "-aq", "--filter", "label=parachute.helper=true", "--filter", "label=parachute.transaction")
	if strings.TrimSpace(helpers) != "" {
		env.T.Fatalf("stale helpers remain: %s", helpers)
	}
	_ = container
}
