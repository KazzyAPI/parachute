package e2e

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestREG_001_credentials_from_docker_config(t *testing.T) {
	env := harness.NewEnv(t)
	registry, _, a := authUpdatable(env)
	env.WriteFile("docker-config/config.json", dockerConfig(t, map[string]string{registry.Host: "user:s3cret-value"}), 0o600)
	assertState(t, runOnce(env), a, "would_update", "")
}

func TestREG_001_missing_config_is_unauthorized(t *testing.T) {
	env := harness.NewEnv(t)
	_, _, a := authUpdatable(env)
	assertState(t, runOnce(env), a, "failed", "registry_unauthorized")
}

func TestREG_002_replaced_config_takes_effect_next_cycle(t *testing.T) {
	env := harness.NewEnv(t)
	registry, _, a := authUpdatable(env)
	path := env.WriteFile("docker-config/config.json", dockerConfig(t, map[string]string{registry.Host: "user:wrong"}), 0o600)
	process := startDaemon(env, env.Parachute("--interval", "3", "--porcelain", "v1"))
	entries := func() []harness.Log {
		return findLogs(process.Stdout(), func(entry harness.Log) bool { return entry.Get("container") == a })
	}
	env.Eventually(30*time.Second, "failing cycle", func() bool { return len(entries()) > 0 })
	if first := entries()[0]; first.Get("reason") != "registry_unauthorized" {
		t.Fatalf("first cycle: %s", first.Raw)
	}
	replacement := env.WriteFile("config-replacement.json", dockerConfig(t, map[string]string{registry.Host: "user:s3cret-value"}), 0o600)
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	env.Eventually(30*time.Second, "succeeding cycle", func() bool {
		all := entries()
		return all[len(all)-1].Get("state") == "would_update"
	})
}

func TestREG_004_exact_host_entry_is_used(t *testing.T) {
	env := harness.NewEnv(t)
	registry, _, a := authUpdatable(env)
	env.WriteFile("docker-config/config.json", dockerConfig(t, map[string]string{"https://" + registry.Host + "/v2/": "user:s3cret-value"}), 0o600)
	assertState(t, runOnce(env), a, "would_update", "")
}

func TestREG_004_other_host_entry_is_not_used(t *testing.T) {
	env := harness.NewEnv(t)
	registry, _, a := authUpdatable(env)
	localhost := "localhost:" + strings.TrimPrefix(registry.Host, "127.0.0.1:")
	env.WriteFile("docker-config/config.json", dockerConfig(t, map[string]string{localhost: "user:s3cret-value"}), 0o600)
	assertState(t, runOnce(env), a, "failed", "registry_unauthorized")
}

func TestREG_003_docker_hub_key_forms(t *testing.T) {
	env := harness.NewEnv(t)
	hub := env.FakeDockerHub()
	image := env.Tag(fixture(env), "docker.io/library/"+env.ID+"-fixture:1")
	env.RunContainer("a", image, harness.ContainerOptions{})
	for index, key := range []string{"https://index.docker.io/v1/", "index.docker.io", "docker.io"} {
		configDir := filepath.Dir(env.WriteFile("hub-config-"+strconv.Itoa(index)+"/config.json", dockerConfig(t, map[string]string{key: "hubuser:hubpass"}), 0o644))
		result := env.RunParachuteContainer("parachute-"+strconv.Itoa(index), harness.ContainerRun{
			Args:   []string{"--run-once", "--porcelain", "v1", "--scope", env.Scope()},
			Env:    map[string]string{"HTTPS_PROXY": hub.ProxyURL(), "SSL_CERT_FILE": "/hub/ca.pem", "DOCKER_CONFIG": "/docker-config"},
			Mounts: []string{filepath.Dir(hub.CAFile) + ":/hub:ro", configDir + ":/docker-config:ro"},
		})
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte("hubuser:hubpass"))
		if tokens := hub.TokenAuthorizations(); len(tokens) <= index || tokens[index] != want {
			t.Fatalf("key %s: token requests carried %v, want %s\n%s", key, tokens, want, result.Stderr)
		}
	}
}
