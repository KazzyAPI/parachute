package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestVER_004_exec_returns_when_deadline_expires_while_command_runs(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/exec") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"Id":"exec1"}`))
		case strings.HasSuffix(r.URL.Path, "/start"):
			conn, buffer, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			defer func() { _ = conn.Close() }()
			_, _ = buffer.WriteString("HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
			_ = buffer.Flush()
			<-release
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	docker, err := NewDocker(Endpoint{Host: "tcp://" + strings.TrimPrefix(server.URL, "http://"), APIVersion: "1.47"})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := docker.Exec(context.Background(), "c1", []string{"sh", "-c", "sleep 1000"}, nil, 200*time.Millisecond, 1000)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error after the deadline")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("exec did not return after its deadline")
	}
}
