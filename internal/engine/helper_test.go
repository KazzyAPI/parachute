package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHelperConfigClearsImageEntrypoint(t *testing.T) {
	config, hostConfig := helperConfig(HelperSpec{Image: "parachute", Command: []string{"sh", "-c", "true"}})
	if len(config.Entrypoint) != 1 || config.Entrypoint[0] != "" {
		t.Fatalf("entrypoint %q does not reset the image entrypoint", config.Entrypoint)
	}
	if !hostConfig.ReadonlyRootfs || string(hostConfig.NetworkMode) != "default" {
		t.Fatalf("unexpected host config %+v", hostConfig)
	}
}

func TestRunHelperRemovesHelperWhenStartIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	removed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/create"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"Id":"h1"}`))
		case strings.HasSuffix(r.URL.Path, "/start"):
			cancel()
			w.WriteHeader(http.StatusInternalServerError)
		case r.Method == http.MethodDelete:
			close(removed)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	docker, err := NewDocker(Endpoint{Host: "tcp://" + strings.TrimPrefix(server.URL, "http://"), APIVersion: "1.47"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := docker.RunHelper(ctx, HelperSpec{Image: "x"}); err == nil {
		t.Fatal("expected start error")
	}
	select {
	case <-removed:
	case <-time.After(2 * time.Second):
		t.Fatal("helper was not removed")
	}
}
