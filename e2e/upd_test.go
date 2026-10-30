package e2e

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func armedOnce(env *harness.Env, args ...string) *harness.Result {
	env.T.Helper()
	base := []string{"--run-once", "--armed", "--delay-days", "0", "--protect-databases=false", "--porcelain", "v1", "--helper-image", env.ParachuteImage()}
	return env.Parachute(append(base, args...)...).Run(runTimeout)
}

func TestUPD_003_replacement_keeps_ports_env_mounts_and_name(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	v1 := registry.PushSpec("app", "1", harness.ImageSpec{Version: "1", Env: map[string]string{"A": "1"}})
	network := env.Docker("network", "create", env.Name("net"))
	a := env.RunContainer("a", v1, harness.ContainerOptions{
		Extra:  []string{"-p", "8080:3000", "-e", "A=1", "--mount", "type=bind,source=" + t.TempDir() + ",destination=/data", "--mount", "type=volume,source=" + env.Name("vol") + ",destination=/var", "--memory", "64m", "--restart", "unless-stopped", "--network", network, "--network-alias", "web"},
		Labels: map[string]string{"x": "y"},
	})
	registry.PushSpec("app", "1", harness.ImageSpec{Version: "2", Env: map[string]string{"A": "1"}})
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
	if !strings.Contains(inspect, `"HostPort": "8080"`) || !strings.Contains(inspect, `"A=1"`) || !strings.Contains(inspect, `"x": "y"`) {
		t.Fatalf("missing preserved configuration:\n%s", inspect)
	}
}

func TestUPD_004_inherited_env_follows_new_image(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	v1 := registry.PushSpec("app", "1", harness.ImageSpec{Version: "1", Env: map[string]string{"VERSION": "1"}})
	a := env.RunContainer("a", v1, harness.ContainerOptions{})
	registry.PushSpec("app", "1", harness.ImageSpec{Version: "2", Env: map[string]string{"VERSION": "2"}})
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
	a := env.RunContainer("a", v1, harness.ContainerOptions{Extra: []string{"-e", "VERSION=custom"}})
	registry.PushSpec("app", "1", harness.ImageSpec{Version: "2", Env: map[string]string{"VERSION": "2"}})
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
	v1 := registry.PushSpec("app", "1", harness.ImageSpec{Base: "busybox:1.37", Version: "1", Volumes: []string{"/data"}, Cmd: []string{"sh", "-c", "sleep 3600"}})
	a := env.RunContainer("a", v1, harness.ContainerOptions{})
	registry.PushSpec("app", "1", harness.ImageSpec{Base: "busybox:1.37", Version: "2", Volumes: []string{"/data"}, Cmd: []string{"sh", "-c", "sleep 3600"}})
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
	proxy := registry.Proxy(harness.ProxyOptions{})
	a := env.RunContainer("a", env.Rehome(v1, proxy.Host), harness.ContainerOptions{})
	registry.PushSpec("fixture/app", "1", harness.ImageSpec{Version: "2"})
	result := armedOnce(env)
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
	env := harness.NewEnv(t)
	a, signalFile, _ := gatedFailure(t, env, "a")
	origID, origImage := env.ContainerState(a)
	done := runArmedAsync(env, "--backup", "none")
	renamedID := waitRenamedOriginal(env, a)
	if renamedID != origID {
		t.Fatalf("renamed container id %s, want original %s", renamedID, origID)
	}
	if running := strings.TrimSpace(env.Docker("inspect", "--format", "{{.State.Running}}", renamedID)); running != "false" {
		t.Fatalf("renamed original running=%q, want stopped", running)
	}
	if !strings.Contains(env.ImageList(), origImage) {
		t.Fatalf("original image %s no longer present", origImage)
	}
	replacement := strings.TrimSpace(env.Docker("ps", "-q", "--filter", "name="+a, "--filter", "label=parachute.transaction"))
	txn := strings.TrimSpace(env.Docker("inspect", "--format", "{{index .Config.Labels \"parachute.transaction\"}}", replacement))
	suffix := txn[len(txn)-6:]
	if name := strings.TrimSpace(env.Docker("inspect", "--format", "{{.Name}}", renamedID)); name != "/"+a+"-parachute-"+suffix {
		t.Fatalf("renamed container %q, want /%s-parachute-%s", name, a, suffix)
	}
	if err := os.WriteFile(signalFile, []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertState(t, <-done, a, "rolled_back", "verify_failed")
}

func TestUPD_020_rejects_pulled_image_without_approved_digest(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	v1 := registry.PushSpec("fixture/app", "1", harness.ImageSpec{Version: "1"})
	v1Manifest := registry.Manifest("fixture/app", "1")
	proxy := registry.Proxy(harness.ProxyOptions{
		Rewrite: func(request *http.Request, header http.Header, body []byte) []byte {
			if request.Method != http.MethodGet || !isManifest(request) || !strings.Contains(request.URL.Path, "/manifests/sha256:") {
				return body
			}
			digest := request.URL.Path[strings.LastIndex(request.URL.Path, "/")+1:]
			header.Set("Docker-Content-Digest", digest)
			return v1Manifest
		},
	})
	a := env.RunContainer("a", env.Rehome(v1, proxy.Host), harness.ContainerOptions{})
	id, image := env.ContainerState(a)
	registry.PushSpec("fixture/app", "1", harness.ImageSpec{Version: "2"})
	result := armedOnce(env)
	assertState(t, result, a, "failed", "pull_failed")
	assertUnchanged(env, a, id, image)
}

func TestUPD_005_bookkeeping_labels_on_compose_service(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	image := registry.PushSpec("app", "1", harness.ImageSpec{Version: "1"})
	project := env.Name("shop")
	a := env.RunContainer("a", image, harness.ContainerOptions{Labels: map[string]string{
		"com.docker.compose.project": project,
		"com.docker.compose.service": "web",
	}})
	registry.PushSpec("app", "1", harness.ImageSpec{Version: "2"})
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
