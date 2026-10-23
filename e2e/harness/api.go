package harness

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type API struct {
	env    *Env
	Port   int
	client *http.Client
}

type APIRequest struct {
	Method        string
	Path          string
	Authorization string
	ForwardedFor  string
	Headers       map[string]string
	Body          string
	Host          string
}

type APIResponse struct {
	Status int
	Header http.Header
	Body   string
}

func (r *APIResponse) JSON() map[string]any {
	var body map[string]any
	if err := json.Unmarshal([]byte(r.Body), &body); err != nil {
		return nil
	}
	return body
}

func (r *APIResponse) Text() string {
	var text strings.Builder
	for name, values := range r.Header {
		text.WriteString(name + ": " + strings.Join(values, ", ") + "\n")
	}
	return text.String() + "\n" + r.Body
}

func Bearer(token string) string { return "Bearer " + token }

func Basic(user, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password))
}

func (e *Env) DockerHost() string { return suite.dockerHost }

func (e *Env) API(port int) *API {
	transport := &http.Transport{MaxIdleConnsPerHost: 16}
	api := &API{env: e, Port: port, client: &http.Client{Transport: transport, Timeout: 3 * time.Minute}}
	e.T.Cleanup(transport.CloseIdleConnections)
	return api
}

func (a *API) address(host string) string {
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, strconv.Itoa(a.Port))
}

func (a *API) WaitReady(timeout time.Duration) {
	a.env.T.Helper()
	a.env.Eventually(timeout, "HTTP API on port "+strconv.Itoa(a.Port), func() bool {
		conn, err := net.DialTimeout("tcp", a.address(""), time.Second)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	})
}

func (a *API) Do(request APIRequest) *APIResponse {
	a.env.T.Helper()
	response, err := a.Try(request)
	if err != nil {
		a.env.T.Fatalf("%s %s: %v", request.Method, request.Path, err)
	}
	return response
}

func (a *API) Try(request APIRequest) (*APIResponse, error) {
	method := request.Method
	if method == "" {
		method = http.MethodGet
	}
	outgoing, err := http.NewRequest(method, "http://"+a.address(request.Host)+request.Path, strings.NewReader(request.Body))
	if err != nil {
		return nil, err
	}
	if request.Authorization != "" {
		outgoing.Header.Set("Authorization", request.Authorization)
	}
	if request.ForwardedFor != "" {
		outgoing.Header.Set("X-Forwarded-For", request.ForwardedFor)
	}
	for name, value := range request.Headers {
		outgoing.Header.Set(name, value)
	}
	response, err := a.client.Do(outgoing)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	return &APIResponse{Status: response.StatusCode, Header: response.Header, Body: string(body)}, nil
}

func (a *API) DialRaw() net.Conn {
	a.env.T.Helper()
	conn, err := net.DialTimeout("tcp", a.address(""), 5*time.Second)
	if err != nil {
		a.env.T.Fatal(err)
	}
	a.env.T.Cleanup(func() { _ = conn.Close() })
	return conn
}
