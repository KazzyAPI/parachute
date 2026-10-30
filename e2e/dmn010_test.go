package e2e

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func parseStatusJSON(t *testing.T, text string) map[string]any {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal([]byte(text), &document); err != nil {
		t.Fatalf("status is not JSON: %v\n%s", err, text)
	}
	return document
}

func twoDistinctEngineHosts(env *harness.Env) string {
	a := env.DockerHost()
	b := env.DistinctEngineProxy()
	return "a=" + a + ",b=" + b.Endpoint
}

func shopStack(env *harness.Env) string {
	return env.Name("shop")
}

func hostEntriesFor(result *harness.Result, container string) map[string]map[string]any {
	entries := map[string]map[string]any{}
	for _, object := range result.Porcelain() {
		if object["type"] == "entry" && object["container"] == container {
			entries[object["host"].(string)] = object
		}
	}
	return entries
}

func TestDMN_010_report_and_porcelain_name_each_host(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	stack := shopStack(env)
	container := env.RunContainer("web", app.ref, harness.ContainerOptions{Labels: composeLabels(stack, "web")})
	app.publishNewer()
	webhook := env.Webhook()
	result := env.Parachute("--run-once", "--porcelain", "v1", "--allow-insecure-tcp", "--notifications-level", "debug",
		"--notification-url", webhook.URL()).
		Setenv("PARACHUTE_HOSTS", twoDistinctEngineHosts(env)).Run(runTimeout)
	assertExit(t, result, 0)
	entries := hostEntriesFor(result, container)
	if len(entries) != 2 {
		t.Fatalf("expected porcelain entries for hosts a and b, got %v", entries)
	}
	for _, host := range []string{"a", "b"} {
		entry, ok := entries[host]
		if !ok {
			t.Fatalf("no porcelain entry for host %s:\n%s", host, result.Stdout)
		}
		if entry["host"] != host {
			t.Errorf("host %s: entry host %v", host, entry["host"])
		}
		if entry["state"] != "would_update" {
			t.Errorf("host %s: state %v, want would_update", host, entry["state"])
		}
		if entry["stack"] != stack {
			t.Errorf("host %s: stack %v, want %s", host, entry["stack"], stack)
		}
	}
	env.Eventually(30*time.Second, "cycle report", func() bool { return len(webhook.Reports()) > 0 })
	report := webhook.Reports()[0].Message
	assertContains(t, report, "a\n  "+stack+"\n", "b\n  "+stack+"\n")
}

func TestDMN_010_status_names_each_host(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	stack := shopStack(env)
	container := env.RunContainer("web", app.ref, harness.ContainerOptions{Labels: composeLabels(stack, "web")})
	app.publishNewer()
	result := env.Parachute("status", "--json", "--allow-insecure-tcp").
		Setenv("PARACHUTE_HOSTS", twoDistinctEngineHosts(env)).Run(runTimeout)
	assertExit(t, result, 0)
	document := parseStatusJSON(t, result.Stdout)
	services := map[string]bool{}
	for _, host := range document["hosts"].([]any) {
		entry := host.(map[string]any)
		name := entry["name"].(string)
		for _, stackEntry := range entry["stacks"].([]any) {
			for _, service := range stackEntry.(map[string]any)["services"].([]any) {
				if service.(map[string]any)["container"] == container {
					services[name] = true
				}
			}
		}
	}
	if !services["a"] || !services["b"] || len(services) != 2 {
		t.Errorf("hosts reporting %s: %v, want a and b", container, services)
	}
}

func TestDMN_010_metrics_name_each_host(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	stack := shopStack(env)
	env.RunContainer("web", app.ref, harness.ContainerOptions{Labels: composeLabels(stack, "web")})
	app.publishNewer()
	process, api := startAPIWith(env, func(parachute *harness.Parachute) *harness.Parachute {
		return parachute.Setenv("PARACHUTE_HOSTS", twoDistinctEngineHosts(env))
	}, "--http-api-metrics", "--allow-insecure-tcp")
	process.WaitForLog(60*time.Second, "first cycle", func(entry harness.Log) bool { return entry.Msg() == "cycle finished" })
	metrics := api.Do(harness.APIRequest{Path: "/v1/metrics", Authorization: harness.Bearer(apiToken)})
	assertResponse(t, metrics, 200)
	assertContains(t, metrics.Body, `host="a"`, `host="b"`, `parachute_host_up{host="a"} 1`, `parachute_host_up{host="b"} 1`)
}

func TestDMN_010_shop_entries_have_distinct_transaction_ids_per_host(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	stack := shopStack(env)
	engines := map[string]*harness.HTTPEngineProxy{"a": env.DistinctEngineProxy(), "b": env.DistinctEngineProxy()}
	containers := map[string]string{}
	for host, proxy := range engines {
		proxy.OnlyContainersLabelled("e2e-host", host)
		labels := composeLabels(stack, "web")
		labels["e2e-host"] = host
		containers[host] = env.RunContainer("web-"+host, app.ref, harness.ContainerOptions{Labels: labels})
	}
	app.publishNewer()
	result := env.Parachute("--run-once", "--armed", "--backup", "none", "--protect-databases=false", "--delay-days", "0",
		"--verify-stable", "1s", "--helper-image", env.ParachuteImage(), "--porcelain", "v1", "--allow-insecure-tcp").
		Setenv("PARACHUTE_HOSTS", "a="+engines["a"].Endpoint+",b="+engines["b"].Endpoint).Run(runTimeout)
	assertExit(t, result, 0)
	entries := map[string]map[string]any{}
	for host, container := range containers {
		entries[host] = hostEntriesFor(result, container)[host]
		if entries[host] == nil {
			t.Fatalf("no entry for %s on host %s:\n%s", container, host, result.Stdout)
		}
		if entries[host]["stack"] != stack {
			t.Errorf("host %s stack %v, want %s", host, entries[host]["stack"], stack)
		}
	}
	txnA, txnB := transactionField(entries["a"]), transactionField(entries["b"])
	if txnA == "" || txnB == "" {
		t.Fatalf("entries must carry a transaction ID: a=%q b=%q", txnA, txnB)
	}
	if txnA == txnB {
		t.Errorf("transaction IDs must differ per host, both %s", txnA)
	}
}

func transactionField(entry map[string]any) string {
	value, ok := entry["txn"]
	if !ok || value == nil {
		return ""
	}
	return value.(string)
}
