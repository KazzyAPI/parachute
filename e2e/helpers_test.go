package e2e

import (
	"encoding/base64"
	"encoding/json"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

const runTimeout = 2 * time.Minute

func runOnce(env *harness.Env, args ...string) *harness.Result {
	env.T.Helper()
	return env.Parachute(append([]string{"--run-once", "--porcelain", "v1"}, args...)...).Run(runTimeout)
}

func assertCandidates(t *testing.T, result *harness.Result, want []string, notWant []string) {
	t.Helper()
	got := result.Candidates()
	for _, name := range want {
		if !slices.Contains(got, name) {
			t.Errorf("expected candidate %s, candidates were %v", name, got)
		}
	}
	for _, name := range notWant {
		if slices.Contains(got, name) {
			t.Errorf("did not expect candidate %s, candidates were %v", name, got)
		}
	}
}

func assertState(t *testing.T, result *harness.Result, container, state, reason string) map[string]any {
	t.Helper()
	entry := result.Entry(container)
	if entry == nil {
		t.Fatalf("no porcelain entry for %s:\n%s", container, result.Stdout)
	}
	if entry["state"] != state {
		t.Errorf("%s: state %v, want %s", container, entry["state"], state)
	}
	if reason == "" && entry["reason"] != nil || reason != "" && entry["reason"] != reason {
		t.Errorf("%s: reason %v, want %q", container, entry["reason"], reason)
	}
	return entry
}

func assertExit(t *testing.T, result *harness.Result, code int) {
	t.Helper()
	if result.Code != code {
		t.Fatalf("exit code %d, want %d\nstderr:\n%s", result.Code, code, result.Stderr)
	}
}

func assertContains(t *testing.T, text string, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if !strings.Contains(text, part) {
			t.Errorf("expected output to contain %q, got:\n%s", part, text)
		}
	}
}

func findLogs(logs []harness.Log, match func(harness.Log) bool) []harness.Log {
	var found []harness.Log
	for _, entry := range logs {
		if match(entry) {
			found = append(found, entry)
		}
	}
	return found
}

type updatable struct {
	registry *harness.Registry
	ref      string
}

func newUpdatable(env *harness.Env, repo string) updatable {
	env.T.Helper()
	registry := env.Registry()
	return updatable{registry: registry, ref: registry.Push(repo, "1", "1")}
}

func (u updatable) publishNewer() {
	u.registry.Push(refRepo(u.ref), "1", "2")
}

func refRepo(ref string) string {
	path := ref[strings.LastIndex(ref, "/")+1:]
	return path[:strings.Index(path, ":")]
}

func assertUnchanged(env *harness.Env, container, id, image string) {
	env.T.Helper()
	gotID, gotImage := env.ContainerState(container)
	if gotID != id || gotImage != image {
		env.T.Errorf("%s changed: id %s -> %s, image %s -> %s", container, id, gotID, image, gotImage)
	}
}

func startDaemon(env *harness.Env, parachute *harness.Parachute) *harness.Process {
	env.T.Helper()
	process := parachute.Start()
	env.T.Cleanup(func() {
		if process.Running() {
			process.Signal(syscall.SIGTERM)
			process.Wait(30 * time.Second)
		}
	})
	return process
}

func cycleStart(t *testing.T, cycleID string) time.Time {
	t.Helper()
	started, err := time.Parse("20060102T150405Z", strings.Split(cycleID, "-")[1])
	if err != nil {
		t.Fatalf("cycle ID %q: %v", cycleID, err)
	}
	return started
}

func announcedNext(t *testing.T, process *harness.Process, index int) time.Time {
	t.Helper()
	announcements := process.WaitForLogs(30*time.Second, "next cycle announcement", index+1, func(entry harness.Log) bool {
		return entry.Msg() == "next cycle scheduled"
	})
	next, err := time.Parse(time.RFC3339, announcements[index].Get("next"))
	if err != nil {
		t.Fatalf("announcement %q: %v", announcements[index].Raw, err)
	}
	return next
}

func firstCycle(t *testing.T, process *harness.Process) harness.Log {
	t.Helper()
	return process.WaitForLog(30*time.Second, "first cycle", func(entry harness.Log) bool { return entry.Msg() == "cycle started" })
}

func assertDuration(t *testing.T, got, want, tolerance time.Duration) {
	t.Helper()
	if got < want-tolerance || got > want+tolerance {
		t.Errorf("duration %s, want %s ± %s", got, want, tolerance)
	}
}

func dockerConfig(entries map[string]string) string {
	auths := map[string]map[string]string{}
	for host, credentials := range entries {
		auths[host] = map[string]string{"auth": base64.StdEncoding.EncodeToString([]byte(credentials))}
	}
	contents, err := json.Marshal(map[string]any{"auths": auths})
	if err != nil {
		panic(err)
	}
	return string(contents)
}

func authUpdatable(env *harness.Env) (*harness.Registry, string, string) {
	env.T.Helper()
	registry := env.AuthRegistry("user", "s3cret-value")
	ref := registry.Push("app", "1", "1")
	a := env.RunContainer("a", ref, harness.ContainerOptions{})
	registry.Push("app", "1", "2")
	return registry, ref, a
}
