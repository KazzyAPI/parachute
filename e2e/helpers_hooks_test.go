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

func hookOrderLabels(mark string) map[string]string {
	labels := map[string]string{}
	for _, hook := range []string{"pre-check", "pre-update", "post-update", "post-check"} {
		labels["parachute.lifecycle."+hook] = "echo -n '" + hook + " ' >> " + mark
	}
	return labels
}

func readHookOrder(t *testing.T, hostPath string) string {
	t.Helper()
	data, err := os.ReadFile(hostPath)
	if err != nil {
		t.Fatalf("read hook order: %v", err)
	}
	return strings.TrimSpace(string(data))
}
