package e2e

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestAPI_012_responses_are_redacted(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.AuthRegistry("user", "s3cret-value")
	ref := registry.Push("app", "1", "1")
	env.WriteFile("docker-config/config.json", dockerConfig(t, map[string]string{registry.Host: "user:s3cret-value"}), 0o600)
	env.RunContainer("db", ref, harness.ContainerOptions{Extra: []string{"-e", "DB_PASSWORD=pw-9z7"}})
	registry.Push("app", "1", "2")
	_, api := startAPI(env, "--http-api-update", "--http-api-metrics", "--http-api-status",
		"--http-api-token", "T0ken-api", "--http-api-status-token", "T0ken-ro",
		"--notification-url", "generic://bob:hunter2@127.0.0.1:1/hook?token=q5ecret")
	responses := []*harness.APIResponse{trigger(api, harness.Bearer("T0ken-api"))}
	for _, request := range []harness.APIRequest{
		{Path: "/v1/status", Authorization: harness.Bearer("T0ken-api")},
		{Path: "/status", Authorization: harness.Basic("any", "T0ken-api")},
		{Path: "/v1/metrics", Authorization: harness.Bearer("T0ken-api")},
		{Method: http.MethodPost, Path: "/v1/update?x=1", Authorization: harness.Bearer("T0ken-api")},
		{Method: http.MethodPost, Path: "/v1/update", Authorization: harness.Bearer("wrong-Tok")},
	} {
		responses = append(responses, api.Do(request))
	}
	secrets := []string{"T0ken-api", "T0ken-ro", "hunter2", "q5ecret", "s3cret-value",
		base64.StdEncoding.EncodeToString([]byte("user:s3cret-value")), "pw-9z7", "wrong-Tok"}
	for index, response := range responses {
		text := response.Text()
		for _, secret := range secrets {
			if strings.Contains(text, secret) {
				t.Errorf("response %d contains %q:\n%s", index, secret, text)
			}
		}
	}
}

func TestSTS_003_status_page_is_self_contained_html(t *testing.T) {
	env := harness.NewEnv(t)
	image := fixture(env)
	services := []string{env.RunContainer("a", image, harness.ContainerOptions{}), env.RunContainer("b", image, harness.ContainerOptions{})}
	process, api := startAPI(env, "--http-api-status", "--http-api-status-token", "status-token")
	waitCycleFinished(process, 1)
	for _, password := range []string{apiToken, "status-token"} {
		response := api.Do(harness.APIRequest{Path: "/status", Authorization: harness.Basic("any", password)})
		assertResponse(t, response, http.StatusOK)
		if contentType := response.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
			t.Errorf("Content-Type %q", contentType)
		}
		assertContains(t, response.Body, services...)
		for _, external := range []string{"<script src", "<link"} {
			if strings.Contains(response.Body, external) {
				t.Errorf("page contains %q", external)
			}
		}
	}
}
