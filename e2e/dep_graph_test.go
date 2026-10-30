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

func TestDEP_003_worker_verify_failure_rolls_back_web_in_one_transaction(t *testing.T) {
	env := harness.NewEnv(t)
	if _, err := env.DockerMayFail("compose", "version"); err != nil {
		t.Skip("docker compose is not installed")
	}
	webApp := newUpdatable(env, "web")
	workerApp := newUpdatable(env, "worker")
	project := env.Name("shop")
	compose := env.WriteFile("compose/compose.yaml", `services:
  web:
    image: `+webApp.ref+`
    labels: {parachute.scope: "`+env.Scope()+`", parachute-e2e: "`+env.ID+`"}
  worker:
    image: `+workerApp.ref+`
    labels: {parachute.scope: "`+env.Scope()+`", parachute-e2e: "`+env.ID+`", parachute.verify.stable: "3s"}
`, 0o644)
	composeUp(env, project, compose)
	_ = os.Remove(compose)
	web := project + "-web-1"
	worker := project + "-worker-1"
	webID, webImage := env.ContainerState(web)
	workerID, workerImage := env.ContainerState(worker)
	webApp.publishNewer()
	workerApp.registry.PushSpec("worker", "1", harness.ImageSpec{Verify: true, Version: "2", Env: map[string]string{"EXIT_AFTER": "1"}})
	result := armedOnce(env, "--verify-stable", "3s")
	webEntry := assertState(t, result, web, "rolled_back", "verify_failed")
	workerEntry := assertState(t, result, worker, "rolled_back", "verify_failed")
	if webEntry["txn"] != workerEntry["txn"] {
		t.Fatalf("txn web %v worker %v", webEntry["txn"], workerEntry["txn"])
	}
	assertUnchanged(env, web, webID, webImage)
	assertUnchanged(env, worker, workerID, workerImage)
}

func TestDEP_004_label_depends_on_stops_dependents_first(t *testing.T) {
	env := harness.NewEnv(t)
	u := newUpdatable(env, "app")
	image := u.ref
	b := env.RunContainer("b", image, harness.ContainerOptions{})
	c := env.RunContainer("c", image, harness.ContainerOptions{})
	a := env.RunContainer("a", image, harness.ContainerOptions{Labels: map[string]string{"parachute.depends-on": b + "," + c}})
	beforeCStarted := containerStartedAt(env, c)
	beforeCRestarts := containerRestartCount(env, c)
	rec := env.StartEventRecorder("")
	u.publishNewer()
	armedOnce(env, "--verify-stable", "1s")
	waitEventOrder(env, rec, 2*time.Minute, "die "+a, "die "+b, "die "+c, "start "+b, "start "+c, "start "+a)
	assertTimingUnchanged(env, c, beforeCStarted, beforeCRestarts)
}

func TestDEP_004_compose_depends_on_orders_stop_and_start(t *testing.T) {
	env := harness.NewEnv(t)
	if _, err := env.DockerMayFail("compose", "version"); err != nil {
		t.Skip("docker compose is not installed")
	}
	dbApp := newUpdatable(env, "db")
	appApp := newUpdatable(env, "svc")
	project := env.Name("shop")
	compose := env.WriteFile("compose/compose.yaml", `services:
  db:
    image: `+dbApp.ref+`
    labels: {parachute.scope: "`+env.Scope()+`", parachute-e2e: "`+env.ID+`"}
  app:
    image: `+appApp.ref+`
    depends_on: [db]
    labels: {parachute.scope: "`+env.Scope()+`", parachute-e2e: "`+env.ID+`"}
`, 0o644)
	composeUp(env, project, compose)
	_ = os.Remove(compose)
	app := project + "-app-1"
	db := project + "-db-1"
	rec := env.StartEventRecorder("")
	dbApp.publishNewer()
	armedOnce(env, "--verify-stable", "1s")
	waitEventOrder(env, rec, 2*time.Minute, "die "+app, "die "+db, "start "+db, "start "+app)
}

func TestDEP_004_parachute_depends_on_ignores_docker_link(t *testing.T) {
	env := harness.NewEnv(t)
	u := newUpdatable(env, "app")
	image := u.ref
	db := env.RunContainer("db", image, harness.ContainerOptions{})
	cache := env.RunContainer("cache", image, harness.ContainerOptions{})
	web := env.RunContainer("web", image, harness.ContainerOptions{
		Labels: map[string]string{"parachute.depends-on": cache},
		Extra:  []string{"--link", db + ":db"},
	})
	before := containerStartedAt(env, web)
	u.publishNewer()
	armedOnce(env, "--name", db, "--verify-stable", "1s")
	if containerStartedAt(env, web) != before {
		t.Fatalf("web was stopped for a linked db update while parachute.depends-on is set")
	}
}

func TestDEP_005_linked_db_restarts_before_web(t *testing.T) {
	env := harness.NewEnv(t)
	u := newUpdatable(env, "app")
	image := u.ref
	db := env.RunContainer("db", image, harness.ContainerOptions{})
	web := env.RunContainer("web", image, harness.ContainerOptions{Extra: []string{"--link", db + ":db"}})
	rec := env.StartEventRecorder("")
	u.publishNewer()
	result := armedOnce(env, "--name", db, "--verify-stable", "1s")
	assertState(t, result, db, "updated", "")
	assertState(t, result, web, "updated", "")
	waitEventOrder(env, rec, 2*time.Minute, "die "+web, "die "+db, "start "+db, "start "+web)
}

func TestDEP_005_service_healthy_waits_before_dependent_starts(t *testing.T) {
	env := harness.NewEnv(t)
	if _, err := env.DockerMayFail("compose", "version"); err != nil {
		t.Skip("docker compose is not installed")
	}
	health := []string{"CMD", "/fixture", "exit", "0"}
	dbApp := newUpdatable(env, "db")
	appApp := newUpdatable(env, "svc")
	dbRef := dbApp.registry.PushSpec("db", "1", harness.ImageSpec{Verify: true, Version: "1", Healthcheck: health})
	project := env.Name("shop")
	compose := env.WriteFile("compose/compose.yaml", `services:
  db:
    image: `+dbRef+`
    labels: {parachute.scope: "`+env.Scope()+`", parachute-e2e: "`+env.ID+`"}
  app:
    image: `+appApp.ref+`
    depends_on:
      db:
        condition: service_healthy
    labels: {parachute.scope: "`+env.Scope()+`", parachute-e2e: "`+env.ID+`"}
`, 0o644)
	composeUp(env, project, compose)
	_ = os.Remove(compose)
	app := project + "-app-1"
	db := project + "-db-1"
	rec := env.StartStatusEventRecorder("")
	dbApp.registry.PushSpec("db", "1", harness.ImageSpec{
		Verify: true, Version: "2", Healthcheck: health, Env: map[string]string{"FIXTURE_HEALTH": "starting"},
	})
	armedOnce(env, "--verify-stable", "1s")
	waitEventOrder(env, rec, 2*time.Minute, "health_status healthy "+db, "start "+app)
}

func TestDEP_006_unrelated_container_unchanged(t *testing.T) {
	env := harness.NewEnv(t)
	u := newUpdatable(env, "app")
	image := u.ref
	db := env.RunContainer("db", image, harness.ContainerOptions{})
	cache := env.RunContainer("cache", image, harness.ContainerOptions{})
	web := env.RunContainer("web", image, harness.ContainerOptions{Extra: []string{"--link", db + ":db"}})
	cacheStarted := containerStartedAt(env, cache)
	cacheRestarts := containerRestartCount(env, cache)
	dbStarted := containerStartedAt(env, db)
	dbRestarts := containerRestartCount(env, db)
	u.publishNewer()
	result := armedOnce(env, "--name", web, "--verify-stable", "1s")
	assertState(t, result, web, "updated", "")
	assertTimingUnchanged(env, cache, cacheStarted, cacheRestarts)
	assertTimingUnchanged(env, db, dbStarted, dbRestarts)
}

func TestDEP_007_network_mode_dependent_recreated(t *testing.T) {
	env := harness.NewEnv(t)
	u := newUpdatable(env, "app")
	image := u.ref
	app := env.RunContainer("app", image, harness.ContainerOptions{})
	sidecar := env.RunContainer("sidecar", image, harness.ContainerOptions{Extra: []string{"--network", "container:" + app}})
	beforeID, beforeImage := env.ContainerState(sidecar)
	u.publishNewer()
	result := armedOnce(env, "--name", app, "--verify-stable", "1s")
	assertState(t, result, app, "updated", "")
	afterID, afterImage := env.ContainerState(sidecar)
	if afterID == beforeID {
		t.Fatalf("sidecar id unchanged")
	}
	if afterImage != beforeImage {
		t.Fatalf("sidecar image changed %s -> %s", beforeImage, afterImage)
	}
	if ref := strings.TrimSpace(env.Docker("inspect", "--format", "{{.HostConfig.NetworkMode}}", sidecar)); !strings.HasSuffix(ref, app) {
		t.Fatalf("sidecar network mode %q, want container:%s", ref, app)
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
	newRef := u.registry.Push("app", "1", "2")
	result := armedOnce(env, "--verify-stable", "1s")
	one := project + "-worker-1"
	two := project + "-worker-2"
	oneEntry := assertState(t, result, one, "updated", "")
	twoEntry := assertState(t, result, two, "updated", "")
	if oneEntry["txn"] != twoEntry["txn"] {
		t.Fatalf("txn %v %v", oneEntry["txn"], twoEntry["txn"])
	}
	for _, name := range []string{one, two} {
		if got := strings.TrimSpace(env.Docker("inspect", "--format", "{{.Config.Image}}", name)); got != newRef {
			t.Errorf("%s image %s, want %s", name, got, newRef)
		}
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
	newRef := u.registry.Push(refRepo(u.ref), "1", "2")
	result := armedOnce(env, "--verify-stable", "1s")
	assertState(t, result, project+"-web-1", "updated", "")
	out := env.Docker("compose", "-p", project, "ps", "--format", "{{.Name}} {{.Image}}")
	if !strings.Contains(out, project+"-web-1") || !strings.Contains(out, newRef) {
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
	two := project + "-worker-2"
	three := project + "-worker-3"
	rec := env.StartEventRecorder("")
	armedOnce(env, "--rolling-restart", "--verify-stable", "1s")
	waitEventOrder(env, rec, 2*time.Minute,
		"die "+one, "start "+one,
		"die "+two, "start "+two,
		"die "+three, "start "+three,
	)
}
