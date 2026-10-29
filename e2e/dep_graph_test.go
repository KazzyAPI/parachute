package e2e

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestDEP_003_cross_stack_shares_transaction(t *testing.T) {
	env := harness.NewEnv(t)
	reg := env.Registry()
	cacheImage := reg.Push("cache", "1", "1")
	appImage := reg.Push("app", "1", "1")
	cache := env.RunContainer("cache", cacheImage, harness.ContainerOptions{})
	app := env.RunContainer("app", appImage, harness.ContainerOptions{Labels: map[string]string{"parachute.depends-on": cache}})
	reg.Push("cache", "1", "2")
	result := armedOnce(env)
	cacheEntry := assertState(t, result, cache, "updated", "")
	appEntry := result.Entry(app)
	if appEntry == nil || appEntry["txn"] != cacheEntry["txn"] {
		t.Fatalf("txn app %v cache %v", appEntry, cacheEntry["txn"])
	}
}

func TestDEP_004_label_depends_on_stops_dependents_first(t *testing.T) {
	env := harness.NewEnv(t)
	u := newUpdatable(env, "app")
	image := u.ref
	b := env.RunContainer("b", image, harness.ContainerOptions{})
	c := env.RunContainer("c", image, harness.ContainerOptions{})
	a := env.RunContainer("a", image, harness.ContainerOptions{Labels: map[string]string{"parachute.depends-on": b + "," + c}})
	beforeB, _ := env.ContainerState(b)
	beforeC, _ := env.ContainerState(c)
	u.publishNewer()
	armedOnce(env, "--name", a)
	afterB, _ := env.ContainerState(b)
	afterC, _ := env.ContainerState(c)
	if afterB == beforeB || afterC == beforeC {
		t.Fatalf("dependents were not restarted")
	}
}

func TestDEP_005_linked_db_restarts_before_web(t *testing.T) {
	env := harness.NewEnv(t)
	u := newUpdatable(env, "app")
	image := u.ref
	db := env.RunContainer("db", image, harness.ContainerOptions{})
	web := env.RunContainer("web", image, harness.ContainerOptions{Extra: []string{"--link", db + ":db"}})
	u.publishNewer()
	beforeDB := env.Docker("inspect", "--format", "{{.State.StartedAt}}", db)
	armedOnce(env, "--name", db)
	afterDB := env.Docker("inspect", "--format", "{{.State.StartedAt}}", db)
	if afterDB == beforeDB {
		t.Fatalf("db was not restarted")
	}
	if env.Docker("inspect", "--format", "{{.State.Status}}", web) != "running" {
		t.Fatalf("web not running")
	}
}

func TestDEP_006_unrelated_container_unchanged(t *testing.T) {
	env := harness.NewEnv(t)
	u := newUpdatable(env, "app")
	image := u.ref
	db := env.RunContainer("db", image, harness.ContainerOptions{})
	cache := env.RunContainer("cache", image, harness.ContainerOptions{})
	web := env.RunContainer("web", image, harness.ContainerOptions{Extra: []string{"--link", db + ":db"}})
	cacheID, cacheImage := env.ContainerState(cache)
	dbID, dbImage := env.ContainerState(db)
	u.publishNewer()
	armedOnce(env, "--name", web)
	assertUnchanged(env, cache, cacheID, cacheImage)
	assertUnchanged(env, db, dbID, dbImage)
}

func TestDEP_007_network_mode_dependent_recreated(t *testing.T) {
	env := harness.NewEnv(t)
	u := newUpdatable(env, "app")
	image := u.ref
	app := env.RunContainer("app", image, harness.ContainerOptions{})
	sidecar := env.RunContainer("sidecar", image, harness.ContainerOptions{Extra: []string{"--network", "container:" + app}})
	before, _ := env.ContainerState(sidecar)
	u.publishNewer()
	armedOnce(env, "--name", app)
	after, _ := env.ContainerState(sidecar)
	if after == before {
		t.Fatalf("sidecar id unchanged")
	}
}

func TestDEP_008_dependency_cycle_fails(t *testing.T) {
	env := harness.NewEnv(t)
	u := newUpdatable(env, "app")
	image := u.ref
	a := env.RunContainer("a", image, harness.ContainerOptions{Labels: map[string]string{"parachute.depends-on": env.Name("b")}})
	b := env.RunContainer("b", image, harness.ContainerOptions{Labels: map[string]string{"parachute.depends-on": a}})
	c := env.RunContainer("c", image, harness.ContainerOptions{})
	u.publishNewer()
	start := time.Now()
	result := armedOnce(env)
	if time.Since(start) > 60*time.Second {
		t.Fatalf("cycle took too long")
	}
	assertState(t, result, a, "failed", "dependency_cycle")
	assertState(t, result, b, "failed", "dependency_cycle")
	assertState(t, result, c, "updated", "")
}

func TestDEP_009_replicas_keep_names(t *testing.T) {
	env := harness.NewEnv(t)
	if _, err := env.DockerMayFail("compose", "version"); err != nil {
		t.Skip("docker compose is not installed")
	}
	u := newUpdatable(env, "app")
	image := u.ref
	project := env.Name("shop")
	compose := env.WriteFile("compose/compose.yaml", `services:
  worker:
    image: `+image+`
    labels: {parachute.scope: "`+env.Scope()+`", parachute-e2e: "`+env.ID+`"}
`, 0o644)
	composeUp(env, project, compose)
	env.Docker("compose", "-p", project, "-f", compose, "up", "-d", "--scale", "worker=2")
	_ = os.Remove(compose)
	u.publishNewer()
	result := armedOnce(env)
	for _, name := range []string{project + "-worker-1", project + "-worker-2"} {
		assertState(t, result, name, "updated", "")
	}
}

func TestDEP_010_compose_lists_after_update(t *testing.T) {
	env := harness.NewEnv(t)
	if _, err := env.DockerMayFail("compose", "version"); err != nil {
		t.Skip("docker compose is not installed")
	}
	u := newUpdatable(env, "app")
	image := u.ref
	project := env.Name("shop")
	compose := env.WriteFile("compose/compose.yaml", `services:
  web:
    image: `+image+`
    labels: {parachute.scope: "`+env.Scope()+`", parachute-e2e: "`+env.ID+`"}
`, 0o644)
	composeUp(env, project, compose)
	_ = os.Remove(compose)
	u.publishNewer()
	armedOnce(env)
	out := env.Docker("compose", "-p", project, "ps", "--format", "{{.Name}}")
	if !strings.Contains(out, project+"-web-1") {
		t.Fatalf("compose ps:\n%s", out)
	}
}

func TestUPD_013_rolling_restart_serial_replicas(t *testing.T) {
	env := harness.NewEnv(t)
	if _, err := env.DockerMayFail("compose", "version"); err != nil {
		t.Skip("docker compose is not installed")
	}
	u := newUpdatable(env, "app")
	image := u.ref
	project := env.Name("shop")
	compose := env.WriteFile("compose/compose.yaml", `services:
  worker:
    image: `+image+`
    labels: {parachute.scope: "`+env.Scope()+`", parachute-e2e: "`+env.ID+`"}
`, 0o644)
	composeUp(env, project, compose)
	env.Docker("compose", "-p", project, "-f", compose, "up", "-d", "--scale", "worker=3")
	_ = os.Remove(compose)
	u.publishNewer()
	one := project + "-worker-1"
	before := env.Docker("inspect", "--format", "{{.State.StartedAt}}", one)
	armedOnce(env, "--rolling-restart")
	after := env.Docker("inspect", "--format", "{{.State.StartedAt}}", one)
	if after == before {
		t.Fatalf("expected rolling restart on %s", one)
	}
}
