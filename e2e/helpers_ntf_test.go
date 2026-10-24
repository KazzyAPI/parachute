package e2e

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

const failingNotificationURL = "generic://bob:hunter2@127.0.0.1:1/hook?token=q5ecret"

type plantedSecrets struct {
	args      []string
	secrets   []string
	container string
}

func plantSecrets(env *harness.Env) plantedSecrets {
	env.T.Helper()
	registry := env.AuthRegistry("user", "s3cret-value")
	ref := registry.Push("app", "1", "1")
	container := env.RunContainer("a", ref, harness.ContainerOptions{Extra: []string{"-e", "DB_PASSWORD=pw-9z7"}})
	registry.Push("app", "1", "2")
	env.WriteFile("docker-config/config.json", dockerConfig(env.T, map[string]string{registry.Host: "user:s3cret-value"}), 0o600)
	return plantedSecrets{
		args: []string{"--http-api-token", "T0ken-api", "--http-api-status-token", "T0ken-ro", "--notification-url", failingNotificationURL},
		secrets: []string{"T0ken-api", "T0ken-ro", "hunter2", "q5ecret", "s3cret-value",
			base64.StdEncoding.EncodeToString([]byte("user:s3cret-value")), "pw-9z7"},
		container: container,
	}
}

func assertNoSecrets(t *testing.T, where, text string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if strings.Contains(text, secret) {
			t.Errorf("%s contains %q:\n%s", where, secret, text)
		}
	}
}

func assertNotificationsHaveNoSecrets(t *testing.T, webhook *harness.Webhook, secrets ...string) {
	t.Helper()
	for _, notification := range webhook.Received() {
		assertNoSecrets(t, "notification", notification.Title+"\n"+notification.Message, secrets...)
	}
}

func challengeRegistry(env *harness.Env, realm func(self string) string, tokenStatus int, tokenBody string) string {
	env.T.Helper()
	server := httptest.NewUnstartedServer(nil)
	self := server.Listener.Addr().String()
	server.Config.Handler = http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/token" {
			http.Error(response, tokenBody, tokenStatus)
			return
		}
		response.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm=%q,service="e2e",scope="repository:app:pull"`, realm(self)))
		http.Error(response, "unauthorized", http.StatusUnauthorized)
	})
	server.Start()
	env.T.Cleanup(server.Close)
	return self
}

func challengedContainer(env *harness.Env, host string) string {
	env.T.Helper()
	return env.RunContainer("a", env.Rehome(fixture(env), host), harness.ContainerOptions{})
}
