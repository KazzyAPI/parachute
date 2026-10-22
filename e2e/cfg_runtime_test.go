package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestCFG_008_json_logs(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	result := runOnce(env, "--log-format", "json", "--debug")
	logs := result.Logs()
	if len(logs) == 0 {
		t.Fatal("no log lines")
	}
	for _, entry := range logs {
		if !entry.JSON() || entry.Time().IsZero() || entry.Level() == "" || entry.Msg() == "" {
			t.Errorf("log line is not JSON with time, level and msg: %s", entry.Raw)
		}
	}
	for _, entry := range findLogs(logs, func(entry harness.Log) bool { return entry.Msg() == "container checked" }) {
		for _, key := range []string{"cycle", "host", "stack", "service", "container"} {
			if entry.Get(key) == "" {
				t.Errorf("container line misses %s: %s", key, entry.Raw)
			}
		}
	}
}

func TestCFG_008_no_color_removes_escape_codes(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once", "--log-format", "pretty").Setenv("NO_COLOR", "1").Run(runTimeout)
	assertExit(t, result, 0)
	if strings.Contains(result.Stderr, "\x1b[") {
		t.Errorf("output contains colour codes:\n%q", result.Stderr)
	}
}

func TestCFG_009_failed_candidate_exits_1(t *testing.T) {
	env := harness.NewEnv(t)
	image := env.Tag(fixture(env), "127.0.0.1:1/"+env.ID+"/offline:1")
	env.RunContainer("a", image, harness.ContainerOptions{})
	assertExit(t, runOnce(env), 1)
}

func TestCFG_009_sigterm_while_idle_exits_0(t *testing.T) {
	env := harness.NewEnv(t)
	process := startDaemon(env, env.Parachute())
	process.WaitForLog(30*time.Second, "cycle finished", func(entry harness.Log) bool { return entry.Msg() == "cycle finished" })
	signalled := time.Now()
	process.Signal(syscall.SIGTERM)
	if code := process.Wait(5 * time.Second); code != 0 {
		t.Fatalf("exit code %d, want 0", code)
	}
	if elapsed := time.Since(signalled); elapsed > 5*time.Second {
		t.Fatalf("took %s to exit", elapsed)
	}
}

func TestCFG_010_missing_state_file_is_created(t *testing.T) {
	env := harness.NewEnv(t)
	assertExit(t, env.Parachute("--run-once").Run(runTimeout), 0)
	contents, err := os.ReadFile(filepath.Join(env.StateDir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(contents, &document); err != nil || document["format"] != "parachute.state/v1" {
		t.Fatalf("state.json is not parachute.state/v1: %v %s", err, contents)
	}
}

func TestCFG_010_corrupt_state_file_is_kept(t *testing.T) {
	env := harness.NewEnv(t)
	path := env.WriteFile("state/state.json", "{broken", 0o644)
	result := env.Parachute("--run-once").Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, path)
	if contents, err := os.ReadFile(path); err != nil || string(contents) != "{broken" {
		t.Fatalf("state file changed: %q %v", contents, err)
	}
}

func TestCFG_010_unwritable_state_directory_is_startup_error(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write read-only directories")
	}
	env := harness.NewEnv(t)
	if err := os.Chmod(env.StateDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(env.StateDir, 0o755) })
	result := env.Parachute("--run-once").Run(runTimeout)
	assertExit(t, result, 2)
	assertContains(t, result.Stderr, filepath.Join(env.StateDir, "state.json"))
}

func TestCFG_010_state_file_survives_sigkill(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	env.RunContainer("a", app.ref, harness.ContainerOptions{})
	env.RunContainer("b", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	path := filepath.Join(env.StateDir, "state.json")
	for round := range 20 {
		process := env.Parachute("--interval", "1").Start()
		time.Sleep(time.Duration(300+round*97%900) * time.Millisecond)
		process.Signal(syscall.SIGKILL)
		process.Wait(10 * time.Second)
		contents, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		var document map[string]any
		if err != nil || json.Unmarshal(contents, &document) != nil {
			t.Fatalf("round %d: state.json does not parse: %v %q", round, err, contents)
		}
	}
}
