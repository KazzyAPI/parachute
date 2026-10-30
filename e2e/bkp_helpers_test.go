package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

func transactionLabel(env *harness.Env, container string) string {
	env.T.Helper()
	return strings.TrimSpace(env.Docker("inspect", "--format", "{{index .Config.Labels \"parachute.transaction\"}}", container))
}

func porcelainField(entry map[string]any, key string) string {
	if value, ok := entry[key].(string); ok {
		return value
	}
	return ""
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

func firstMount(env *harness.Env, manifest map[string]any) map[string]any {
	env.T.Helper()
	mounts, _ := manifest["mounts"].([]any)
	if len(mounts) == 0 {
		env.T.Fatalf("snapshot manifest has no mounts: %v", manifest)
	}
	record, _ := mounts[0].(map[string]any)
	if record == nil {
		env.T.Fatalf("snapshot manifest mount is not an object: %v", mounts[0])
	}
	return record
}

func firstSnapshotDir(env *harness.Env, storeDir string) string {
	env.T.Helper()
	snaps := listSnapshotDirs(env, storeDir)
	if len(snaps) == 0 {
		env.T.Fatalf("no snapshot in %s", storeDir)
	}
	return snaps[0]
}

func backupVolumePath(env *harness.Env, volume string) string {
	env.T.Helper()
	return strings.TrimSpace(env.Docker("volume", "inspect", "--format", "{{.Mountpoint}}", volume))
}

func assertReasonDetail(t *testing.T, result *harness.Result, container string, parts ...string) {
	t.Helper()
	detail := result.Detail(container)
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

func assertManifestDestinations(t *testing.T, manifest map[string]any, key string, want, omit []string) {
	t.Helper()
	got := manifestStrings(manifest, key, "destination")
	for _, dest := range want {
		if !slices.Contains(got, dest) {
			t.Fatalf("%s destinations %v missing %s", key, got, dest)
		}
	}
	for _, dest := range omit {
		if slices.Contains(got, dest) {
			t.Fatalf("%s destinations %v must not include %s", key, got, dest)
		}
	}
}

func assertManifestTransactionMatches(t *testing.T, env *harness.Env, manifest map[string]any, container string) {
	t.Helper()
	want := transactionLabel(env, container)
	got, _ := manifest["transaction"].(string)
	if got != want || want == "" {
		t.Fatalf("manifest transaction %q, want %q from %s", got, want, container)
	}
}

func assertManifestSchema(t *testing.T, manifest map[string]any, snapshotID string) {
	t.Helper()
	if manifest["format"] != "parachute.snapshot/v1" {
		t.Fatalf("format %v, want parachute.snapshot/v1", manifest["format"])
	}
	if manifest["id"] != snapshotID {
		t.Fatalf("id %v, want %s", manifest["id"], snapshotID)
	}
	for _, key := range []string{"host", "stack", "created", "containers"} {
		if manifest[key] == nil || manifest[key] == "" {
			t.Fatalf("manifest missing %s", key)
		}
	}
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
	out := env.Docker("run", "--rm", "--entrypoint", "ls", "-v", storeDir+":"+storeDir+":ro", env.FixtureV2Image(), "-1", storeDir)
	var snaps []string
	for _, name := range strings.Fields(out) {
		if strings.HasPrefix(name, "snap-") {
			snaps = append(snaps, name)
		}
	}
	return snaps
}

func stackSnapshotIDs(env *harness.Env, storeDir, stack string) []string {
	env.T.Helper()
	var ids []string
	for _, id := range listSnapshotDirs(env, storeDir) {
		manifest := readManifest(env, storeDir, id)
		if manifest["stack"] == stack {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

func assertRetentionKeepsNewest(t *testing.T, env *harness.Env, storeDir, stack string, created []string, keep int) {
	t.Helper()
	if len(created) < keep {
		t.Fatalf("created %d snapshots, need at least %d", len(created), keep)
	}
	want := slices.Clone(created[len(created)-keep:])
	got := stackSnapshotIDs(env, storeDir, stack)
	if len(got) != keep {
		t.Fatalf("retention: got %v (%d), want %d newest %v", got, len(got), keep, want)
	}
	for _, id := range want {
		if !slices.Contains(got, id) {
			t.Fatalf("retention missing %s in %v", id, got)
		}
	}
	for _, id := range created[:len(created)-keep] {
		if slices.Contains(got, id) {
			t.Fatalf("retention kept old snapshot %s", id)
		}
	}
}

func verifyFailLabels(signalDir string) map[string]string {
	cmd := fmt.Sprintf("while [ ! -f %s ]; do sleep 0.1; done; exit 1", filepath.Join(signalDir, "go"))
	return map[string]string{"parachute.verify.cmd": cmd}
}

func verifyFailUpdate(env *harness.Env, repo string) string {
	env.T.Helper()
	return pushV2(env, repo, "1", harness.ImageSpec{Version: "2"})
}

func waitSnapshotDuringVerify(env *harness.Env, container, signalFile string, tamper func(snapID string)) {
	env.T.Helper()
	name := env.Name(container)
	var snap string
	env.Eventually(2*time.Minute, "snapshot label during verify", func() bool {
		out, err := env.DockerMayFail("inspect", "--format", "{{index .Config.Labels \"parachute.snapshot\"}}", name)
		snap = out
		return err == nil && strings.HasPrefix(snap, "snap-")
	})
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
		logs, _ := env.DockerMayFail("logs", container)
		if !strings.Contains(logs, "PostgreSQL init process complete") {
			return false
		}
		_, err := env.DockerMayFail("exec", container, "pg_isready", "-U", "postgres")
		return err == nil
	})
}

func counterContainerCommand() []string {
	script := "i=0; while true; do i=$((i+1)); echo $i > /data/c; sleep 0.1; done"
	return []string{"--entrypoint", "sh", "-c", script}
}

func parseContainerEvent(line string) (action, name string, ok bool) {
	fields := strings.Fields(line)
	if len(fields) < 3 || fields[0] != "container" {
		return "", "", false
	}
	return fields[1], fields[2], true
}

func assertStopModeSnapshotOrder(t *testing.T, events []string, appName string) {
	t.Helper()
	dieIndex := -1
	for index, line := range events {
		action, name, parsed := parseContainerEvent(line)
		if parsed && action == "die" && name == appName {
			dieIndex = index
			break
		}
	}
	if dieIndex < 0 {
		t.Fatalf("no die event for %s in %v", appName, events)
	}
	for index := dieIndex + 1; index < len(events); index++ {
		action, name, parsed := parseContainerEvent(events[index])
		if !parsed || action != "start" {
			continue
		}
		if name == appName {
			t.Fatalf("app %s started before snapshot helper in %v", appName, events)
		}
		return
	}
	t.Fatalf("no helper start after die for %s in %v", appName, events)
}

func assertLiveModeSnapshotOrder(t *testing.T, events []string, appName string) {
	t.Helper()
	appDie := -1
	for index, line := range events {
		action, name, parsed := parseContainerEvent(line)
		if parsed && action == "die" && name == appName {
			appDie = index
		}
	}
	if appDie < 0 {
		t.Fatalf("no die event for %s in %v", appName, events)
	}
	helperName := ""
	helperFinished := -1
	for index, line := range events {
		if index >= appDie {
			break
		}
		action, name, parsed := parseContainerEvent(line)
		if !parsed || name == appName {
			continue
		}
		if action == "start" && helperName == "" {
			helperName = name
		}
		if helperName != "" && name == helperName && (action == "die" || action == "destroy") {
			helperFinished = index
		}
	}
	if helperName == "" || helperFinished < 0 {
		t.Fatalf("helper lifecycle before app die not found in %v", events)
	}
	if helperFinished >= appDie {
		t.Fatalf("helper %s finished at %d after app die at %d in %v", helperName, helperFinished, appDie, events)
	}
}

func assertNoDieEvent(t *testing.T, events []string, appName string) {
	t.Helper()
	for _, line := range events {
		action, name, parsed := parseContainerEvent(line)
		if parsed && action == "die" && name == appName {
			t.Fatalf("unexpected die for %s in %v", appName, events)
		}
	}
}

func assertInsufficientSpaceDetail(t *testing.T, result *harness.Result, container string) {
	t.Helper()
	detail := result.Detail(container)
	var numbers []string
	for _, field := range strings.Fields(detail) {
		if strings.Trim(field, "0123456789") == "" && field != "" {
			numbers = append(numbers, field)
		}
	}
	if len(numbers) < 2 {
		t.Fatalf("detail %q missing estimated and free byte counts", detail)
	}
}

func assertNoPreUpdateHookLogs(t *testing.T, result *harness.Result) {
	t.Helper()
	combined := result.Stderr + result.Stdout
	for _, needle := range []string{"pre-update hook", "lifecycle.pre-update", "pre_update_hook"} {
		if strings.Contains(strings.ToLower(combined), strings.ToLower(needle)) {
			t.Fatalf("unexpected pre-update hook activity: %q", needle)
		}
	}
}

func assertNoSnapshotCreated(t *testing.T, env *harness.Env, store string) {
	t.Helper()
	if dirs := listSnapshotDirs(env, store); len(dirs) > 0 {
		t.Fatalf("snapshot directories created: %v", dirs)
	}
}

func assertMySQLDumpSecretRedacted(t *testing.T, env *harness.Env, containerEvents, execEvents []string, result *harness.Result, secret string) {
	t.Helper()
	lines := append([]string{}, containerEvents...)
	lines = append(lines, execEvents...)
	for _, log := range result.Logs() {
		lines = append(lines, log.Raw)
	}
	lines = append(lines, result.Stderr)
	for _, line := range lines {
		if strings.Contains(line, secret) {
			t.Fatalf("secret leaked in %q", line)
		}
	}
	seen := map[string]bool{}
	for _, line := range execEvents {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "exec_create" {
			continue
		}
		id := fields[1]
		if seen[id] {
			continue
		}
		seen[id] = true
		spec := env.Docker("inspect", id)
		if strings.Contains(spec, secret) {
			t.Fatalf("secret in exec inspect %s", id)
		}
	}
}

func assertRestoreVerifyFail(t *testing.T, env *harness.Env, result *harness.Result, reason string) {
	t.Helper()
	assertExit(t, result, 1)
	path := filepath.Join(env.StateDir, "restore-verify.json")
	if raw, err := os.ReadFile(path); err == nil {
		var document map[string]any
		if err := json.Unmarshal(raw, &document); err == nil {
			if stacks, ok := document["stacks"].([]any); ok && len(stacks) > 0 {
				if stack, ok := stacks[0].(map[string]any); ok {
					if stack["result"] != "fail" {
						t.Fatalf("restore-verify result %v, want fail", stack["result"])
					}
					if stack["reason"] != reason {
						t.Fatalf("restore-verify reason %v, want %s", stack["reason"], reason)
					}
					return
				}
			}
		}
	}
	if !strings.Contains(result.Stderr, reason) {
		t.Fatalf("stderr missing reason %q:\n%s", reason, result.Stderr)
	}
}

func volumeMarker(env *harness.Env, volume, relative, content string) string {
	env.T.Helper()
	env.Docker("run", "--rm", "--entrypoint", "sh", "-v", volume+":/data", env.FixtureV2Image(), "-c", "printf '%s' '"+content+"' > /data/"+relative)
	return content
}

func readVolumeMarker(env *harness.Env, volume, relative string) string {
	env.T.Helper()
	return strings.TrimSpace(env.Docker("run", "--rm", "--entrypoint", "cat", "-v", volume+":/data", env.FixtureV2Image(), "/data/"+relative))
}

func assertWarnSystemMountSkipped(t *testing.T, result *harness.Result, container, destination string) {
	t.Helper()
	for _, log := range result.Logs() {
		if log.Msg() == "system mount skipped" && log.Get("container") == container && log.Get("destination") == destination && log.Get("source") != "" {
			return
		}
	}
	t.Fatalf("missing warn for system mount skipped %s %s", container, destination)
}

func manifestSkippedRecord(manifest map[string]any, destination string) map[string]any {
	raw, _ := manifest["skipped"].([]any)
	for _, item := range raw {
		record, _ := item.(map[string]any)
		if record["destination"] == destination {
			return record
		}
	}
	return nil
}

func observeSnapshotHelpers(env *harness.Env, volume string, run func()) {
	env.T.Helper()
	seen := map[string]struct{}{}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			out, _ := env.DockerMayFail("ps", "-aq", "--filter", "label=parachute.helper=true", "--filter", "volume="+volume)
			for _, id := range strings.Fields(out) {
				txn := strings.TrimSpace(env.Docker("inspect", "--format", "{{index .Config.Labels \"parachute.transaction\"}}", id))
				if txn == "" {
					continue
				}
				if _, ok := seen[id]; !ok {
					seen[id] = struct{}{}
					assertHelperInspect(env.T, env, id, volume)
				}
			}
			time.Sleep(25 * time.Millisecond)
		}
	}()
	run()
	close(stop)
	<-done
	if len(seen) == 0 {
		env.T.Fatal("no snapshot helper observed during cycle")
	}
}

func assertHelperInspect(t *testing.T, env *harness.Env, id, dataVolume string) {
	t.Helper()
	raw := env.Docker("inspect", id)
	var specs []map[string]any
	if err := json.Unmarshal([]byte(raw), &specs); err != nil || len(specs) == 0 {
		t.Fatalf("inspect helper %s: %v", id, err)
	}
	host, _ := specs[0]["HostConfig"].(map[string]any)
	if host == nil {
		t.Fatal("helper missing HostConfig")
	}
	if privileged, _ := host["Privileged"].(bool); privileged {
		t.Fatal("helper privileged")
	}
	if readonly, _ := host["ReadonlyRootfs"].(bool); !readonly {
		t.Fatal("helper rootfs not read-only")
	}
	assertCapConstraints(t, host)
	assertNamespaceNotHost(t, host, "NetworkMode")
	assertNamespaceNotHost(t, host, "PidMode")
	assertNamespaceNotHost(t, host, "IpcMode")
	assertNamespaceNotHost(t, host, "UTSMode")
	if bindings, _ := host["PortBindings"].(map[string]any); len(bindings) > 0 {
		t.Fatalf("helper published ports %v", bindings)
	}
	mounts, _ := specs[0]["Mounts"].([]any)
	if len(mounts) == 0 {
		t.Fatal("helper has no mounts")
	}
	var readWrite, readOnly int
	for _, item := range mounts {
		mount, _ := item.(map[string]any)
		if mount == nil {
			continue
		}
		source, _ := mount["Source"].(string)
		if strings.Contains(source, "docker.sock") {
			t.Fatalf("helper mounted docker socket %s", source)
		}
		rw, _ := mount["RW"].(bool)
		if rw {
			readWrite++
		} else {
			readOnly++
		}
		if name, _ := mount["Name"].(string); name == dataVolume && rw {
			t.Fatal("data volume mount must be read-only on helper")
		}
	}
	if readOnly == 0 || readWrite == 0 {
		t.Fatalf("helper mounts want read-only sources and read-write snapshot dir, got rw=%d ro=%d", readWrite, readOnly)
	}
}

func assertCapConstraints(t *testing.T, host map[string]any) {
	t.Helper()
	allowed := map[string]bool{"CHOWN": true, "DAC_OVERRIDE": true, "FOWNER": true, "FSETID": true}
	drop, _ := host["CapDrop"].([]any)
	foundAll := false
	for _, item := range drop {
		if fmt.Sprint(item) == "ALL" {
			foundAll = true
		}
	}
	if !foundAll {
		t.Fatalf("CapDrop %v missing ALL", drop)
	}
	if add, ok := host["CapAdd"].([]any); ok {
		for _, item := range add {
			name := fmt.Sprint(item)
			if !allowed[name] {
				t.Fatalf("CapAdd %s not allowed", name)
			}
		}
	}
	opts, _ := host["SecurityOpt"].([]any)
	hasNoNewPriv := false
	for _, item := range opts {
		if strings.Contains(fmt.Sprint(item), "no-new-privileges") {
			hasNoNewPriv = true
		}
	}
	if !hasNoNewPriv {
		t.Fatalf("SecurityOpt %v missing no-new-privileges", opts)
	}
}

func assertNamespaceNotHost(t *testing.T, host map[string]any, field string) {
	t.Helper()
	value := strings.ToLower(fmt.Sprint(host[field]))
	if value == "host" {
		t.Fatalf("helper %s is host", field)
	}
}

func createFakeHelper(env *harness.Env, labels map[string]string) string {
	env.T.Helper()
	args := []string{"run", "-d", "--label", "parachute-e2e=" + env.ID, "--label", "parachute.helper=true"}
	for key, value := range labels {
		args = append(args, "--label", key+"="+value)
	}
	args = append(args, "--entrypoint", "sleep", env.FixtureV2Image(), "3600")
	return env.Docker(args...)
}

func replaceArchive(env *harness.Env, storeDir, snapID, file string, shellScript string) {
	env.T.Helper()
	snapDir := filepath.Join(storeDir, snapID)
	env.Docker("run", "--rm", "--entrypoint", "sh", "-v", snapDir+":/snap", env.FixtureV2Image(), "-c", shellScript+" > /snap/"+file)
	data := []byte(env.ReadHostPath(filepath.Join(snapDir, file)))
	sum := sha256.Sum256(data)
	manifest := readManifest(env, storeDir, snapID)
	record := firstMount(env, manifest)
	record["sha256"] = hex.EncodeToString(sum[:])
	record["size"] = float64(len(data))
	encoded, _ := json.Marshal(manifest)
	env.WriteHostPath(filepath.Join(snapDir, "manifest.json"), string(encoded))
}

func killDuringSnapshot(env *harness.Env, volume string, app *updatable, container string) {
	env.T.Helper()
	env.Docker("run", "--rm", "--entrypoint", "sh", "-v", volume+":/data", env.FixtureV2Image(), "-c", "head -c 209715200 /dev/zero > /data/fill")
	app.publishNewer()
	process := env.Parachute("--run-once", "--armed", "--protect-databases=false", "--delay-days", "0",
		"--verify-stable", "1s", "--verify-timeout", "30s", "--helper-image", env.ParachuteImage(), "--porcelain", "v1").Start()
	env.Eventually(2*time.Minute, "snapshot helper", func() bool {
		return strings.TrimSpace(env.Docker("ps", "-q", "--filter", "label=parachute.helper=true", "--filter", "volume="+volume)) != ""
	})
	process.Signal(syscall.SIGKILL)
	process.Wait(30 * time.Second)
	runArmed(env)
	if helpers := strings.Join(env.TransactionContainers(true), " "); helpers != "" {
		env.T.Fatalf("stale helpers remain: %s", helpers)
	}
	_ = container
}
