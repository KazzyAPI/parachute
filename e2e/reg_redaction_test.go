package e2e

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestREG_006_credentials_never_appear_in_outputs(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	registry, _, a := authUpdatable(env)
	env.WriteFile("docker-config/config.json", dockerConfig(t, map[string]string{registry.Host: "user:s3cret-value"}), 0o600)
	result := notify(env, webhook, "--log-level", "debug")
	assertState(t, result, a, "would_update", "")
	status := env.Parachute("status", "--json").Run(runTimeout)
	assertExit(t, status, 0)
	secrets := []string{"s3cret-value", base64.StdEncoding.EncodeToString([]byte("user:s3cret-value"))}
	for where, text := range map[string]string{"stdout": result.Stdout, "stderr": result.Stderr, "status": status.Stdout} {
		assertNoSecrets(t, where, text, secrets...)
	}
	assertNotificationsHaveNoSecrets(t, webhook, secrets...)
}

func TestREG_006_token_error_body_is_not_shown(t *testing.T) {
	env := harness.NewEnv(t)
	webhook := env.Webhook()
	host := challengeRegistry(env, func(self string) string { return "http://" + self + "/token" }, http.StatusInternalServerError, "s3cret-value MARKER-BODY")
	a := challengedContainer(env, host)
	env.WriteFile("docker-config/config.json", dockerConfig(t, map[string]string{host: "user:s3cret-value"}), 0o600)
	result := notify(env, webhook, "--log-level", "debug")
	assertState(t, result, a, "failed", "registry_unreachable")
	checked := findLogs(result.Logs(), func(entry harness.Log) bool { return entry.Msg() == "container checked" && entry.Get("container") == a })
	if len(checked) != 1 || !strings.Contains(checked[0].Get("detail"), host) || !strings.Contains(checked[0].Get("detail"), "500") {
		t.Errorf("detail does not name %s and 500: %v", host, checked)
	}
	status := env.Parachute("status", "--json").Run(runTimeout)
	assertExit(t, status, 0)
	secrets := []string{"s3cret-value", "MARKER-BODY"}
	for where, text := range map[string]string{"stdout": result.Stdout, "stderr": result.Stderr, "status": status.Stdout} {
		assertNoSecrets(t, where, text, secrets...)
	}
	assertNotificationsHaveNoSecrets(t, webhook, secrets...)
}

func TestREG_006_trace_level_warns_about_credentials(t *testing.T) {
	env := harness.NewEnv(t)
	result := env.Parachute("--run-once", "--log-level", "trace").Run(runTimeout)
	assertExit(t, result, 0)
	warnings := findLogs(result.Logs(), func(entry harness.Log) bool {
		return entry.Level() == "warn" && strings.Contains(entry.Msg(), "credentials")
	})
	if len(warnings) == 0 {
		t.Error("no warn line mentions credentials")
	}
}
