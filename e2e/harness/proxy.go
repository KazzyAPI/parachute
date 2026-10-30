package harness

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ProxyOptions struct {
	RejectHead bool
	Delay      time.Duration
	Handle     func(http.ResponseWriter, *http.Request) bool
	Rewrite    func(request *http.Request, header http.Header, body []byte) []byte
}

type ProxyRequest struct {
	Method        string
	Path          string
	Authorization string
}

type RegistryProxy struct {
	Host     string
	Port     string
	URL      string
	lock     sync.Mutex
	requests []ProxyRequest
}

func (r *Registry) Proxy(options ProxyOptions) *RegistryProxy {
	target, err := url.Parse("http://" + r.Host)
	if err != nil {
		r.env.T.Fatal(err)
	}
	forward := httputil.NewSingleHostReverseProxy(target)
	if options.Rewrite != nil {
		forward.ModifyResponse = func(response *http.Response) error {
			body, err := io.ReadAll(response.Body)
			if closeErr := response.Body.Close(); err == nil {
				err = closeErr
			}
			if err != nil {
				return err
			}
			body = options.Rewrite(response.Request, response.Header, body)
			response.Body = io.NopCloser(bytes.NewReader(body))
			if response.Request.Method != http.MethodHead {
				response.ContentLength = int64(len(body))
				response.Header.Set("Content-Length", strconv.Itoa(len(body)))
			}
			return nil
		}
	}
	proxy := r.env.Server(func(response http.ResponseWriter, request *http.Request) {
		if options.RejectHead && request.Method == http.MethodHead && strings.Contains(request.URL.Path, "/manifests/") {
			http.Error(response, "HEAD not allowed", http.StatusMethodNotAllowed)
			return
		}
		if options.Handle != nil && options.Handle(response, request) {
			return
		}
		time.Sleep(options.Delay)
		forward.ServeHTTP(response, request)
	})
	if suite.desktop {
		r.env.Docker("run", "-d", "--network", "host", "--label", "parachute-e2e="+r.env.ID, "--label", "parachute.enable=false",
			suite.fixtureImage, "forward", proxy.Host, "host.docker.internal:"+proxy.Port)
	}
	return proxy
}

func (e *Env) Server(handler http.HandlerFunc) *RegistryProxy {
	proxy := &RegistryProxy{}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		proxy.lock.Lock()
		proxy.requests = append(proxy.requests, ProxyRequest{Method: request.Method, Path: request.URL.Path, Authorization: request.Header.Get("Authorization")})
		proxy.lock.Unlock()
		handler(response, request)
	}))
	e.T.Cleanup(server.Close)
	proxy.URL = server.URL
	proxy.Host = strings.TrimPrefix(server.URL, "http://")
	proxy.Port = proxy.Host[strings.LastIndex(proxy.Host, ":")+1:]
	return proxy
}

func (p *RegistryProxy) Requests() []ProxyRequest {
	p.lock.Lock()
	defer p.lock.Unlock()
	return append([]ProxyRequest{}, p.requests...)
}
