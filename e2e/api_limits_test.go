package e2e

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestAPI_009_oversized_headers_get_431(t *testing.T) {
	env := harness.NewEnv(t)
	process, api := startAPI(env, "--http-api-update")
	response := api.Do(harness.APIRequest{Method: http.MethodPost, Path: "/v1/update", Authorization: harness.Bearer(apiToken),
		Headers: map[string]string{"X-Pad": strings.Repeat("p", 9<<10)}})
	assertResponse(t, response, http.StatusRequestHeaderFieldsTooLarge)
	assertNoCycle(t, process)
}

func TestAPI_009_oversized_body_gets_413(t *testing.T) {
	env := harness.NewEnv(t)
	process, api := startAPI(env, "--http-api-update")
	response := api.Do(harness.APIRequest{Method: http.MethodPost, Path: "/v1/update", Authorization: harness.Bearer(apiToken),
		Body: strings.Repeat("b", 2<<10)})
	assertResponse(t, response, http.StatusRequestEntityTooLarge)
	assertNoCycle(t, process)
}

func TestAPI_009_incomplete_headers_are_closed_after_10_seconds(t *testing.T) {
	env := harness.NewEnv(t)
	_, api := startAPI(env, "--http-api-status")
	conn := api.DialRaw()
	opened := time.Now()
	if _, err := io.WriteString(conn, "GET /v1/status HTTP/1.1\r\n"); err != nil {
		t.Fatal(err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, conn); err != nil {
		t.Fatalf("connection was not closed: %v", err)
	}
	assertDuration(t, time.Since(opened), 10*time.Second, 5*time.Second)
}
