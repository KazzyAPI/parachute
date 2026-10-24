package e2e

import (
	"encoding/json"
	"strings"
	"testing"

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
	raw := env.ReadHostPath(dir + "/" + snapshotID + "/manifest.json")
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
