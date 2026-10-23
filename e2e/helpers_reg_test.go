package e2e

import (
	"net/http"
	"strings"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func isManifest(request *http.Request) bool {
	return strings.Contains(request.URL.Path, "/manifests/")
}

func manifestGets(proxy *harness.RegistryProxy) []harness.ProxyRequest {
	var gets []harness.ProxyRequest
	for _, request := range proxy.Requests() {
		if request.Method == http.MethodGet && strings.Contains(request.Path, "/manifests/") {
			gets = append(gets, request)
		}
	}
	return gets
}

func proxiedUpdatable(env *harness.Env, options harness.ProxyOptions) (string, *harness.RegistryProxy) {
	env.T.Helper()
	app := newUpdatable(env, "app")
	proxy := app.registry.Proxy(options)
	a := env.RunContainer("a", env.Rehome(app.ref, proxy.Host), harness.ContainerOptions{})
	app.publishNewer()
	return a, proxy
}

func bearerChallenge(realm string) func(http.ResponseWriter, *http.Request) bool {
	return func(response http.ResponseWriter, request *http.Request) bool {
		if !isManifest(request) || request.Header.Get("Authorization") != "" {
			return false
		}
		response.Header().Set("WWW-Authenticate", `Bearer realm="`+realm+`",service="e2e",scope="repository:app:pull"`)
		response.WriteHeader(http.StatusUnauthorized)
		return true
	}
}

func rewriteManifest(method string, change func(header http.Header, body []byte) []byte) func(*http.Request, http.Header, []byte) []byte {
	return func(request *http.Request, header http.Header, body []byte) []byte {
		if request.Method != method || !isManifest(request) {
			return body
		}
		return change(header, body)
	}
}
