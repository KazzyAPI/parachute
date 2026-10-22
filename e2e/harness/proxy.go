package harness

import (
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

type ProxyOptions struct {
	RejectHead bool
	Delay      time.Duration
}

type RegistryProxy struct {
	Host string
	Port string
}

func (r *Registry) Proxy(options ProxyOptions) *RegistryProxy {
	target, err := url.Parse("http://" + r.Host)
	if err != nil {
		r.env.T.Fatal(err)
	}
	forward := httputil.NewSingleHostReverseProxy(target)
	proxy := &RegistryProxy{}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
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
