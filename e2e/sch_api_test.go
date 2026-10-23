package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestSCH_008_api_mode_ignores_the_interval(t *testing.T) {
	env := harness.NewEnv(t)
	env.RunContainer("a", fixture(env), harness.ContainerOptions{})
	process, _ := startAPI(env, "--http-api-update", "--interval", "5")
	time.Sleep(20 * time.Second)
	assertNoCycle(t, process)
	warnings := findLogs(process.Logs(), func(entry harness.Log) bool {
		return entry.Level() == "warn" && strings.Contains(entry.Msg(), "interval") && strings.Contains(entry.Msg(), "ignored")
	})
	if len(warnings) != 1 {
		t.Errorf("expected one warn line saying the interval is ignored, got %d", len(warnings))
	}
}
