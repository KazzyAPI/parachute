package harness

import (
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

type ProxyOptions struct {
	RejectHead bool
	Delay      time.Duration
}

type RegistryProxy struct {
	Host     string
	Port     string
	lock     sync.Mutex
	requests []RegistryRequest
}

func (r *Registry) Proxy(options ProxyOptions) *RegistryProxy {
	target, err := url.Parse("http://" + r.Host)
	if err != nil {
		r.env.T.Fatal(err)
	}
	forward := httputil.NewSingleHostReverseProxy(target)
	proxy := &RegistryProxy{}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		proxy.lock.Lock()
		proxy.requests = append(proxy.requests, RegistryRequest{Method: request.Method, URI: request.URL.RequestURI(), UserAgent: request.UserAgent()})
		proxy.lock.Unlock()
		if options.RejectHead && request.Method == http.MethodHead && strings.Contains(request.URL.Path, "/manifests/") {
			http.Error(response, "HEAD not allowed", http.StatusMethodNotAllowed)
			return
		}
		time.Sleep(options.Delay)
		forward.ServeHTTP(response, request)
	}))
	r.env.T.Cleanup(server.Close)
	proxy.Host = strings.TrimPrefix(server.URL, "http://")
	proxy.Port = proxy.Host[strings.LastIndex(proxy.Host, ":")+1:]
	return proxy
}

func (p *RegistryProxy) Requests() []RegistryRequest {
	p.lock.Lock()
	defer p.lock.Unlock()
	return append([]RegistryRequest{}, p.requests...)
}
