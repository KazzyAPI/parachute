package registry

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type noopHead struct{}

func (noopHead) HeadFailed(context.Context, Reference, error) {}

func testClient(t *testing.T, handler http.Handler) (*Client, Reference) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	host := strings.TrimPrefix(server.URL, "http://")
	ref := Reference{Domain: host, Repository: "app", Tag: "1"}
	return NewClient(server.Client(), NewAuthenticator(server.Client(), "test"), "test", noopHead{}), ref
}

func TestTagsMultiplePages(t *testing.T) {
	var pages atomic.Int32
	client, ref := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch pages.Add(1) {
		case 1:
			w.Header().Set("Link", `</v2/app/tags/list?last=b>; rel="next"`)
			_ = json.NewEncoder(w).Encode(map[string][]string{"tags": {"a", "b"}})
		default:
			_ = json.NewEncoder(w).Encode(map[string][]string{"tags": {"c"}})
		}
	}))
	tags, err := client.Tags(context.Background(), ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(tags, ",") != "a,b,c" || pages.Load() != 2 {
		t.Fatalf("tags %v pages %d", tags, pages.Load())
	}
}

func TestTagsRelativeLink(t *testing.T) {
	var pages atomic.Int32
	client, ref := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pages.Add(1) == 1 {
			w.Header().Set("Link", `</v2/app/tags/list?last=b>; rel="next"`)
			_ = json.NewEncoder(w).Encode(map[string][]string{"tags": {"a"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string][]string{"tags": {"b"}})
	}))
	tags, err := client.Tags(context.Background(), ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(tags, ",") != "a,b" {
		t.Fatalf("tags %v", tags)
	}
}

func TestTagsLinkLoop(t *testing.T) {
	client, ref := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", `</v2/app/tags/list>; rel="next"`)
		_ = json.NewEncoder(w).Encode(map[string][]string{"tags": {"a"}})
	}))
	tags, err := client.Tags(context.Background(), ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0] != "a" {
		t.Fatalf("tags %v", tags)
	}
}

func TestTagsSecondPageError(t *testing.T) {
	var pages atomic.Int32
	client, ref := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pages.Add(1) == 1 {
			w.Header().Set("Link", `</v2/app/tags/list?last=b>; rel="next"`)
			_ = json.NewEncoder(w).Encode(map[string][]string{"tags": {"a"}})
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	_, err := client.Tags(context.Background(), ref, nil)
	var registryErr *Error
	if !errors.As(err, &registryErr) || registryErr.Reason != ReasonUnreachable {
		t.Fatalf("error %v", err)
	}
}

func TestTagsCancelledContext(t *testing.T) {
	started := make(chan struct{})
	block := make(chan struct{})
	client, ref := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("last") == "" {
			w.Header().Set("Link", `</v2/app/tags/list?last=b>; rel="next"`)
			_ = json.NewEncoder(w).Encode(map[string][]string{"tags": {"a"}})
			return
		}
		close(started)
		<-block
		_ = json.NewEncoder(w).Encode(map[string][]string{"tags": {"b"}})
	}))
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := client.Tags(ctx, ref, nil)
		errCh <- err
	}()
	<-started
	cancel()
	err := <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error %v", err)
	}
	close(block)
}
