package e2e

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestREG_005_credential_helper_is_warned_not_run(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	ref := registry.Push("app", "1", "1")
	env.RunContainer("a", ref, harness.ContainerOptions{})
	env.RunContainer("b", ref, harness.ContainerOptions{})
	env.WriteFile("docker-config/config.json", `{"credHelpers":{"`+registry.Host+`":"ecr-login"}}`, 0o600)
	result := runOnce(env)
	warnings := findLogs(result.Logs(), func(entry harness.Log) bool {
		return entry.Level() == "warn" && strings.Contains(entry.Raw, "ecr-login") && strings.Contains(entry.Raw, registry.Host)
	})
	if len(warnings) != 1 {
		t.Errorf("expected one warn line naming ecr-login and %s, got %d\n%s", registry.Host, len(warnings), result.Stderr)
	}
}

func TestREG_007_oversized_manifest_fails_only_that_candidate(t *testing.T) {
	env := harness.NewEnv(t)
	a, _ := proxiedUpdatable(env, harness.ProxyOptions{RejectHead: true, Rewrite: rewriteManifest(http.MethodGet, func(http.Header, []byte) []byte {
		return make([]byte, 5<<20)
	})})
	b := env.RunContainer("b", fixture(env), harness.ContainerOptions{})
	result := runOnce(env)
	assertState(t, result, a, "failed", "registry_unreachable")
	assertState(t, result, b, "fresh", "")
}

func tokenEndpointFails(t *testing.T, token http.HandlerFunc) time.Duration {
	env := harness.NewEnv(t)
	realm := env.Server(token)
	a, _ := proxiedUpdatable(env, harness.ProxyOptions{Handle: bearerChallenge(realm.URL + "/token")})
	started := time.Now()
	result := runOnce(env)
	elapsed := time.Since(started)
	assertState(t, result, a, "failed", "registry_unreachable")
	if len(realm.Requests()) == 0 {
		t.Error("token endpoint was not asked")
	}
	return elapsed
}

func TestREG_007_endless_token_response_fails(t *testing.T) {
	elapsed := tokenEndpointFails(t, func(response http.ResponseWriter, request *http.Request) {
		chunk := bytes.Repeat([]byte("a"), 32<<10)
		for request.Context().Err() == nil {
			if _, err := response.Write(chunk); err != nil {
				return
			}
		}
	})
	if elapsed > 15*time.Second {
		t.Errorf("cycle took %s, want it to stop reading well before the 30 s client timeout", elapsed)
	}
}

func TestREG_007_non_json_token_fails(t *testing.T) {
	tokenEndpointFails(t, func(response http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(response, "not json")
	})
}

func TestREG_007_non_http_realm_fails(t *testing.T) {
	for _, realm := range []string{"file:///etc/passwd", "ftp://127.0.0.1/x"} {
		t.Run(realm, func(t *testing.T) {
			env := harness.NewEnv(t)
			a, _ := proxiedUpdatable(env, harness.ProxyOptions{Handle: bearerChallenge(realm)})
			assertState(t, runOnce(env), a, "failed", "registry_unreachable")
		})
	}
}

func TestREG_008_cross_host_redirect_drops_authorization(t *testing.T) {
	env := harness.NewEnv(t)
	registry, ref, _ := authUpdatable(env)
	recorder := env.Server(func(response http.ResponseWriter, _ *http.Request) { response.WriteHeader(http.StatusNotFound) })
	proxy := registry.Proxy(harness.ProxyOptions{Handle: func(response http.ResponseWriter, request *http.Request) bool {
		if !isManifest(request) || request.Header.Get("Authorization") == "" {
			return false
		}
		http.Redirect(response, request, recorder.URL+request.URL.Path, http.StatusTemporaryRedirect)
		return true
	}})
	env.RunContainer("b", env.Rehome(ref, proxy.Host), harness.ContainerOptions{})
	env.WriteFile("docker-config/config.json", dockerConfig(t, map[string]string{proxy.Host: "user:s3cret-value"}), 0o600)
	runOnce(env)
	requests := recorder.Requests()
	if len(requests) == 0 {
		t.Fatal("the redirect was not followed")
	}
	for _, request := range requests {
		if request.Authorization != "" {
			t.Errorf("recording server received Authorization on %s %s", request.Method, request.Path)
		}
	}
}

func TestREG_008_plain_http_redirect_is_not_followed(t *testing.T) {
	env := harness.NewEnv(t)
	outbound := env.Server(func(response http.ResponseWriter, _ *http.Request) { response.WriteHeader(http.StatusNotFound) })
	a, _ := proxiedUpdatable(env, harness.ProxyOptions{Handle: func(response http.ResponseWriter, request *http.Request) bool {
		if !isManifest(request) {
			return false
		}
		http.Redirect(response, request, "http://other.example.test"+request.URL.Path, http.StatusTemporaryRedirect)
		return true
	}})
	result := env.Parachute("--run-once", "--porcelain", "v1").Setenv("HTTP_PROXY", outbound.URL).Run(runTimeout)
	assertState(t, result, a, "failed", "registry_unreachable")
	if requests := outbound.Requests(); len(requests) > 0 {
		t.Errorf("requests left for other.example.test: %v", requests)
	}
}
