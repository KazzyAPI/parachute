package registry

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type recordingTransport struct {
	requests []*http.Request
}

func (t *recordingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.requests = append(t.requests, request)
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"token":"t"}`)), Request: request}, nil
}

func TestTokenRealmCredentialsNeedHTTPSOrLoopback(t *testing.T) {
	credential := &Credential{Username: "user", Password: "s3cret-value"}
	for realm, allowed := range map[string]bool{
		"http://auth.example.test/token":  false,
		"https://auth.example.test/token": true,
		"http://127.0.0.1:5001/token":     true,
		"http://localhost:5001/token":     true,
	} {
		transport := &recordingTransport{}
		auth := NewAuthenticator(&http.Client{Transport: transport}, "test")
		_, err := auth.Authorization(context.Background(), auth.Parse(`Bearer realm="`+realm+`"`), credential)
		var registryErr *Error
		if allowed != (err == nil) || !allowed && (!errors.As(err, &registryErr) || registryErr.Reason != ReasonUnauthorized) {
			t.Errorf("%s: error %v", realm, err)
		}
		if !allowed && len(transport.requests) > 0 {
			t.Errorf("%s: credentials sent over plain HTTP", realm)
		}
	}
}

func TestTokenRealmOverPlainHTTPStaysUsableAnonymously(t *testing.T) {
	transport := &recordingTransport{}
	auth := NewAuthenticator(&http.Client{Transport: transport}, "test")
	if _, err := auth.Authorization(context.Background(), auth.Parse(`Bearer realm="http://auth.example.test/token"`), nil); err != nil {
		t.Fatal(err)
	}
	if len(transport.requests) != 1 || transport.requests[0].Header.Get("Authorization") != "" {
		t.Errorf("anonymous token request not sent as expected: %d requests", len(transport.requests))
	}
}
