package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestUPD_001_successful_update_logs_phases_in_order(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	result := armedParachute(env, "--log-format", "json").Run(runTimeout)
	assertState(t, result, a, "updated", "")
	var phases, transactions []string
	for _, entry := range result.Logs() {
		phase := entry.Get("phase")
		if phase == "" {
			continue
		}
		if phases = append(phases, phase); phase != "detect" && phase != "policy" {
			transactions = append(transactions, entry.Get("txn"))
		}
	}
	if joined := strings.Join(phases, ","); joined != "detect,policy,snapshot,update,verify,commit" {
		t.Fatalf("phases %s", joined)
	}
	for _, id := range transactions {
		if id == "" || id != transactions[0] {
			t.Fatalf("transaction ids %v", transactions)
		}
	}
}

func TestUPD_001_unchanged_image_starts_no_transaction(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	result := armedParachute(env, "--log-format", "json").Run(runTimeout)
	assertState(t, result, a, "fresh", "")
	for _, entry := range result.Logs() {
		if entry.Get("phase") == "policy" || entry.Get("txn") != "" {
			t.Fatalf("unexpected transaction activity: %s", entry.Raw)
		}
	}
}

func TestUPD_002_transaction_id_is_on_every_log_line_and_the_replacement(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	a := env.RunContainer("a", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	result := armedParachute(env, "--log-format", "json").Run(runTimeout)
	assertState(t, result, a, "updated", "")
	id := ""
	for _, entry := range result.Logs() {
		if entry.Get("container") != a || entry.Get("phase") == "detect" || entry.Get("phase") == "policy" {
			continue
		}
		if id == "" {
			id = entry.Get("txn")
		}
		if entry.Get("txn") == "" || entry.Get("txn") != id {
			t.Fatalf("line without the transaction id %q: %s", id, entry.Raw)
		}
	}
	if label := strings.TrimSpace(env.Docker("inspect", "--format", `{{index .Config.Labels "parachute.transaction"}}`, a)); id == "" || label != id {
		t.Fatalf("label %q, log id %q", label, id)
	}
}

func TestUPD_006_default_stop_timeout_is_ten_seconds(t *testing.T) {
	assertKilledAfterStopTimeout(t, func(p *harness.Parachute) *harness.Parachute { return p }, 10*time.Second)
}

func TestUPD_012_no_restart_leaves_the_replacement_created(t *testing.T) {
	env := harness.NewEnv(t)
	app := newShellUpdatable(env, "upd012")
	a := env.RunContainer("upd012", app.ref, harness.ContainerOptions{})
	app.publishNewer()
	_, before := env.ContainerState(a)
	result := armedRunOnce(env, "--no-restart")
	assertUpdated(t, result, a)
	if status := strings.TrimSpace(env.Docker("inspect", "--format", "{{.State.Status}}", a)); status != "created" {
		t.Fatalf("replacement is %s", status)
	}
	if _, after := env.ContainerState(a); after == before {
		t.Fatal("replacement still uses the old image")
	}
}
