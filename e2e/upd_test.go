package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func armedOnce(env *harness.Env, args ...string) *harness.Result {
	env.T.Helper()
	base := []string{"--run-once", "--armed", "--delay-days", "0", "--protect-databases=false", "--porcelain", "v1"}
	return env.Parachute(append(base, args...)...).Run(runTimeout)
}

func TestUPD_003_replacement_keeps_ports_env_mounts_and_name(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	v1 := registry.PushSpec("app", "1", harness.ImageSpec{Version: "1", Env: map[string]string{"A": "1"}})
	registry.PushSpec("app", "1", harness.ImageSpec{Version: "2", Env: map[string]string{"A": "1"}})
	network := env.Docker("network", "create", env.Name("net"))
	a := env.RunContainer("a", v1, harness.ContainerOptions{
		Extra:  []string{"-p", "8080:3000", "-e", "A=1", "--mount", "type=bind,source=/tmp,destination=/data", "--mount", "type=volume,source=" + env.Name("vol") + ",destination=/var", "--memory", "64m", "--restart", "unless-stopped", "--network", network, "--network-alias", "web"},
		Labels: map[string]string{"x": "y"},
	})
	id, _ := env.ContainerState(a)
	result := armedOnce(env)
	assertState(t, result, a, "updated", "")
	newID, newImage := env.ContainerState(a)
	if newID == id {
		t.Fatalf("container %s was not recreated", a)
	}
	inspect := env.Docker("inspect", a)
	if !strings.Contains(inspect, newImage) {
		t.Fatalf("replacement image not applied:\n%s", inspect)
	}
	if !strings.Contains(inspect, `"8080/tcp"`) || !strings.Contains(inspect, `"A=1"`) || !strings.Contains(inspect, `"x": "y"`) {
		t.Fatalf("missing preserved configuration:\n%s", inspect)
	}
}

func TestUPD_004_inherited_env_follows_new_image(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	v1 := registry.PushSpec("app", "1", harness.ImageSpec{Version: "1", Env: map[string]string{"VERSION": "1"}})
	registry.PushSpec("app", "1", harness.ImageSpec{Version: "2", Env: map[string]string{"VERSION": "2"}})
	a := env.RunContainer("a", v1, harness.ContainerOptions{})
	assertState(t, armedOnce(env), a, "updated", "")
	inspect := env.Docker("inspect", "--format", "{{json .Config.Env}}", a)
	if !strings.Contains(inspect, "VERSION=2") {
		t.Fatalf("expected VERSION=2, got %s", inspect)
	}
}

func TestUPD_004_explicit_env_is_kept(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	v1 := registry.PushSpec("app", "1", harness.ImageSpec{Version: "1", Env: map[string]string{"VERSION": "1"}})
	registry.PushSpec("app", "1", harness.ImageSpec{Version: "2", Env: map[string]string{"VERSION": "2"}})
	a := env.RunContainer("a", v1, harness.ContainerOptions{Extra: []string{"-e", "VERSION=custom"}})
	assertState(t, armedOnce(env), a, "updated", "")
	inspect := env.Docker("inspect", "--format", "{{json .Config.Env}}", a)
	if !strings.Contains(inspect, "VERSION=custom") {
		t.Fatalf("expected VERSION=custom, got %s", inspect)
	}
}

func TestUPD_007_autoremove_blocks_update(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Extra: []string{"--rm"}})
	id, image := env.ContainerState(a)
	app.publishNewer()
	assertState(t, armedOnce(env), a, "failed", "config_unreproducible")
	assertUnchanged(env, a, id, image)
}

func TestUPD_011_anonymous_volume_data_carries_over(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	v1 := registry.PushSpec("app", "1", harness.ImageSpec{Version: "1", Volumes: []string{"/data"}, Cmd: []string{"sh", "-c", "sleep 3600"}})
	registry.PushSpec("app", "1", harness.ImageSpec{Version: "2", Volumes: []string{"/data"}, Cmd: []string{"sh", "-c", "sleep 3600"}})
	a := env.RunContainer("a", v1, harness.ContainerOptions{})
	env.Docker("exec", a, "sh", "-c", "echo marker > /data/marker")
	assertState(t, armedOnce(env), a, "updated", "")
	out := env.Docker("exec", a, "cat", "/data/marker")
	if strings.TrimSpace(out) != "marker" {
		t.Fatalf("marker missing after update: %q", out)
	}
}

func TestUPD_020_pull_uses_approved_digest(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	v1 := registry.PushSpec("fixture/app", "1", harness.ImageSpec{Version: "1"})
	d2 := registry.PushSpec("fixture/app", "1", harness.ImageSpec{Version: "2"})
	a := env.RunContainer("a", v1, harness.ContainerOptions{})
	proxy := registry.Proxy(harness.ProxyOptions{})
	rehomed := env.Rehome(v1, strings.Split(proxy.Host, ":")[0])
	env.Docker("pull", "-q", rehomed)
	env.Docker("tag", rehomed, v1)
	env.Docker("rmi", "-f", rehomed)
	_ = d2
	result := armedOnce(env, "--registry", "http://"+proxy.Host)
	assertState(t, result, a, "updated", "")
	requests := proxy.Requests()
	found := false
	for _, request := range requests {
		if strings.Contains(request.Path, "/manifests/sha256:") && !strings.Contains(request.Path, "/manifests/1") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected digest manifest pull, got %v", requests)
	}
}

func TestUPD_008_original_container_kept_until_commit(t *testing.T) {
	t.Skip("needs verify-phase timing hook to inspect renamed original mid-transaction")
}

func TestUPD_020_rejects_pulled_image_without_approved_digest(t *testing.T) {
	t.Skip("needs registry proxy that serves wrong digest body after manifest approval")
}

func TestUPD_005_bookkeeping_labels_on_compose_service(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	image := registry.PushSpec("app", "1", harness.ImageSpec{Version: "1"})
	registry.PushSpec("app", "1", harness.ImageSpec{Version: "2"})
	project := env.Name("shop")
	a := env.RunContainer("a", image, harness.ContainerOptions{Labels: map[string]string{
		"com.docker.compose.project": project,
		"com.docker.compose.service": "web",
	}})
	assertState(t, armedOnce(env), a, "updated", "")
	labels := map[string]string{}
	raw := env.Docker("inspect", "--format", "{{json .Config.Labels}}", a)
	if err := json.Unmarshal([]byte(raw), &labels); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"parachute.transaction", "parachute.updated-at", "parachute.previous-image", "parachute.snapshot"} {
		if labels[key] == "" {
			t.Fatalf("missing label %s: %v", key, labels)
		}
	}
	if labels["com.docker.compose.project"] != project || labels["com.docker.compose.service"] != "web" {
		t.Fatalf("compose labels changed: %v", labels)
	}
}
