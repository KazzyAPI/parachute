package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func hookVolume(env *harness.Env) (hostDir, mountPath string) {
	env.T.Helper()
	hostDir = env.T.TempDir()
	return hostDir, "/hook-data"
}

func hookContainerExtra(hostDir, mountPath string) []string {
	return []string{"--mount", "type=bind,source=" + hostDir + ",target=" + mountPath}
}

func hookLabel(hook, script string) map[string]string {
	return map[string]string{"parachute.lifecycle." + hook: script}
}

func hookFile(path string) string {
	return filepath.Join(path, "ran")
}

func assertHookFile(t *testing.T, hostPath string, want bool) {
	t.Helper()
	_, err := os.Stat(hostPath)
	got := err == nil
	if got != want {
		if want {
			t.Fatalf("expected hook marker at %s", hostPath)
		}
		t.Fatalf("unexpected hook marker at %s", hostPath)
	}
}

func hookAppender(mount, name string) string {
	return "echo -n '" + name + " ' >> " + hookFile(mount)
}

func allLifecycleHookLabels(mount string) map[string]string {
	return map[string]string{
		"parachute.lifecycle.pre-check":   hookAppender(mount, "pre-check"),
		"parachute.lifecycle.pre-update":  hookAppender(mount, "pre-update"),
		"parachute.lifecycle.post-update": hookAppender(mount, "post-update"),
		"parachute.lifecycle.post-check":  hookAppender(mount, "post-check"),
	}
}

func readHookLog(t *testing.T, hostDir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(hostDir, "ran"))
	if err != nil {
		t.Fatalf("read hook log: %v", err)
	}
	return strings.TrimSpace(string(data))
}

func armedHooks(env *harness.Env, args ...string) []string {
	env.T.Helper()
	base := []string{
		"--armed",
		"--delay-days", "0",
		"--protect-databases=false",
		"--helper-image", env.ParachuteImage(),
		"--enable-lifecycle-hooks",
	}
	return append(base, args...)
}
