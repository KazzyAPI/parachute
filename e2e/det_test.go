package e2e

import (
	"strings"
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func blobGets(requests []harness.RegistryRequest) []harness.RegistryRequest {
	var gets []harness.RegistryRequest
	for _, request := range requests {
		if request.Method == "GET" && strings.Contains(request.URI, "/blobs/") {
			gets = append(gets, request)
		}
	}
	return gets
}

func headWarnings(result *harness.Result) []harness.Log {
	return findLogs(result.Logs(), func(entry harness.Log) bool {
		return entry.Level() == "warn" && strings.Contains(entry.Msg(), "HEAD")
	})
}

func TestDET_001_changed_tag_is_detected(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	assertState(t, runOnce(env), a, "would_update", "")
}

func TestDET_001_unchanged_image_is_fresh(t *testing.T) {
	env := harness.NewEnv(t)
	a := env.RunContainer("a", fixture(env), harness.ContainerOptions{})
	assertState(t, runOnce(env), a, "fresh", "")
}

func TestDET_001_untagged_reference_uses_latest(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	latest := registry.Push("app", "latest", "1")
	untagged := env.Tag(latest, strings.TrimSuffix(latest, ":latest"))
	a := env.RunContainer("a", untagged, harness.ContainerOptions{})
	fresh := runOnce(env)
	assertState(t, fresh, a, "fresh", "")
	registry.Push("app", "latest", "2")
	assertState(t, runOnce(env), a, "would_update", "")
}

func TestDET_002_unchanged_image_downloads_no_blobs(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	env.RunContainer("a", registry.Push("app", "1", "1"), harness.ContainerOptions{})
	assertExit(t, runOnce(env), 0)
	requests := registry.ParachuteRequests()
	if len(requests) == 0 {
		t.Fatal("Parachute did not contact the registry")
	}
	if gets := blobGets(requests); len(gets) > 0 {
		t.Errorf("blob downloads during detection: %v", gets)
	}
}

func TestDET_002_changed_image_unarmed_downloads_no_blobs(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	images := env.ImageList()
	assertState(t, runOnce(env), a, "would_update", "")
	if gets := blobGets(app.registry.ParachuteRequests()); len(gets) > 0 {
		t.Errorf("blob downloads during detection: %v", gets)
	}
	if env.ImageList() != images {
		t.Error("local image list changed")
	}
}

func headRejectingSetup(env *harness.Env, host func(*harness.RegistryProxy) string) (string, *harness.RegistryProxy) {
	env.T.Helper()
	app := newUpdatable(env, "app")
	proxy := app.registry.Proxy(harness.ProxyOptions{RejectHead: true})
	image := env.Rehome(app.ref, host(proxy))
	a := env.RunContainer("a", image, harness.ContainerOptions{})
	app.publishNewer()
	return a, proxy
}

func TestDET_003_get_fallback_detects_quietly_with_never(t *testing.T) {
	env := harness.NewEnv(t)
	a, _ := headRejectingSetup(env, func(proxy *harness.RegistryProxy) string { return proxy.Host })
	result := runOnce(env, "--warn-on-head-failure", "never")
	assertState(t, result, a, "would_update", "")
	if warnings := headWarnings(result); len(warnings) > 0 {
		t.Errorf("unexpected HEAD warnings: %v", warnings[0].Raw)
	}
}

func TestDET_003_always_warns_per_failed_head(t *testing.T) {
	env := harness.NewEnv(t)
	a, proxy := headRejectingSetup(env, func(proxy *harness.RegistryProxy) string { return proxy.Host })
	result := runOnce(env, "--warn-on-head-failure", "always")
	assertState(t, result, a, "would_update", "")
	warnings := headWarnings(result)
	if len(warnings) != 1 || warnings[0].Get("registry") != proxy.Host {
		t.Errorf("expected one HEAD warning naming %s, got %d", proxy.Host, len(warnings))
	}
}

func TestDET_003_auto_is_quiet_for_local_registry(t *testing.T) {
	env := harness.NewEnv(t)
	a, _ := headRejectingSetup(env, func(proxy *harness.RegistryProxy) string { return "localhost:" + proxy.Port })
	result := runOnce(env)
	assertState(t, result, a, "would_update", "")
	if warnings := headWarnings(result); len(warnings) > 0 {
		t.Errorf("unexpected HEAD warnings: %v", warnings[0].Raw)
	}
}

func TestDET_003_offline_registry_fails_only_that_candidate(t *testing.T) {
	env := harness.NewEnv(t)
	healthy := fixture(env)
	offline := env.Rehome(healthy, "127.0.0.1:1")
	a := env.RunContainer("a", offline, harness.ContainerOptions{})
	b := env.RunContainer("b", healthy, harness.ContainerOptions{})
	result := runOnce(env)
	assertState(t, result, a, "failed", "registry_unreachable")
	assertState(t, result, b, "fresh", "")
}

func TestDET_004_no_pull_uses_local_image(t *testing.T) {
	env := harness.NewEnv(t)
	image := fixture(env)
	a := env.RunContainer("a", image, harness.ContainerOptions{})
	env.BuildImage(image, "local-2")
	assertState(t, runOnce(env, "--no-pull"), a, "would_update", "")
}

func TestDET_004_no_pull_ignores_registry(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	original := env.Tag(app.ref, app.ref+"-original")
	app.publishNewer()
	env.Docker("tag", original, app.ref)
	assertState(t, runOnce(env, "--no-pull"), a, "fresh", "")
	if requests := app.registry.ParachuteRequests(); len(requests) > 0 {
		t.Errorf("Parachute contacted the registry: %v", requests)
	}
}

func TestDET_005_digest_pinned_is_skipped(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	digest := env.Docker("inspect", "--format", "{{index .RepoDigests 0}}", app.ref)
	pinned := app.ref[:strings.LastIndex(app.ref, ":")] + digest[strings.Index(digest, "@"):]
	a := env.RunContainer("a", pinned, harness.ContainerOptions{})
	id, image := env.ContainerState(a)
	app.publishNewer()
	assertState(t, runOnce(env), a, "skipped", "digest_pinned")
	assertUnchanged(env, a, id, image)
}

func TestDET_006_never_pushed_image_is_local(t *testing.T) {
	env := harness.NewEnv(t)
	registry := env.Registry()
	local := registry.Ref("local-app", "dev")
	env.BuildImage(local, "dev")
	a := env.RunContainer("a", local, harness.ContainerOptions{})
	assertState(t, runOnce(env), a, "skipped", "local_image")
}

func TestDET_007_held_entry_carries_candidate_digest(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{Labels: map[string]string{"parachute.monitor-only": "true"}})
	app.publishNewer()
	entry := assertState(t, runOnce(env), a, "held", "monitor_only")
	candidate, _ := entry["candidate_digest"].(string)
	if !strings.HasPrefix(candidate, "sha256:") {
		t.Errorf("candidate digest %v", entry["candidate_digest"])
	}
}
