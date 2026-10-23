package e2e

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

const approvalToken = "pol-approval-token"

func cronAt(at time.Time) string {
	at = at.UTC()
	return fmt.Sprintf("%d %d %d %d %d *", at.Second(), at.Minute(), at.Hour(), at.Day(), int(at.Month()))
}

func reportedHoldEnd(t *testing.T, message, reason string) time.Time {
	t.Helper()
	match := regexp.MustCompile(`\(` + reason + `\) until (\S+)`).FindStringSubmatch(message)
	if match == nil {
		t.Fatalf("report has no %s end time:\n%s", reason, message)
	}
	end, err := time.Parse(time.RFC3339, match[1])
	if err != nil {
		t.Fatalf("hold end %q is not RFC 3339: %v", match[1], err)
	}
	return end
}

func entriesFor(process *harness.Process, container string) []harness.Log {
	return findLogs(process.Stdout(), func(entry harness.Log) bool {
		return entry.Get("type") == "entry" && entry.Get("container") == container
	})
}

func waitForEntry(env *harness.Env, process *harness.Process, container string, match func(harness.Log) bool) harness.Log {
	env.T.Helper()
	var found harness.Log
	env.Eventually(90*time.Second, "entry for "+container, func() bool {
		for _, entry := range entriesFor(process, container) {
			if match(entry) {
				found = entry
				return true
			}
		}
		return false
	})
	return found
}

func assertEntry(t *testing.T, entry harness.Log, state, reason string) {
	t.Helper()
	if entry.Get("state") != state || entry.Get("reason") != reason {
		t.Errorf("entry %s, want state %s reason %q", entry.Raw, state, reason)
	}
}

type approvalDaemon struct {
	env     *harness.Env
	port    int
	process *harness.Process
}

func startApprovalDaemon(env *harness.Env, interval int) *approvalDaemon {
	env.T.Helper()
	port := env.FreePort()
	process := startDaemon(env, env.Parachute(
		"--porcelain", "v1", "--interval", strconv.Itoa(interval), "--http-api-update", "--http-api-periodic-polls",
		"--http-api-token", approvalToken, "--http-api-port", strconv.Itoa(port),
	))
	return &approvalDaemon{env: env, port: port, process: process}
}

func (d *approvalDaemon) approve(container string) int {
	d.env.T.Helper()
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/approve?container=%s", d.port, container)
	request, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		d.env.T.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+approvalToken)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		d.env.T.Fatalf("approve %s: %v", container, err)
	}
	if err := response.Body.Close(); err != nil {
		d.env.T.Fatal(err)
	}
	return response.StatusCode
}

func (d *approvalDaemon) approveHeld(container string) string {
	d.env.T.Helper()
	held := waitForEntry(d.env, d.process, container, func(entry harness.Log) bool { return entry.Get("reason") == "approval_required" })
	if code := d.approve(container); code != http.StatusAccepted {
		d.env.T.Fatalf("approve %s: status %d, want 202", container, code)
	}
	return held.Get("candidate_digest")
}
