package harness

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Registry struct {
	env       *Env
	container string
	Host      string
	Port      int
	cliConfig string
}

type RegistryRequest struct {
	Method    string
	URI       string
	UserAgent string
}

func (e *Env) Registry() *Registry {
	return e.startRegistry("", "")
}

func (e *Env) AuthRegistry(user, password string) *Registry {
	return e.startRegistry(user, password)
}

func (e *Env) startRegistry(user, password string) *Registry {
	e.T.Helper()
	port := e.FreePort()
	registry := &Registry{env: e, container: e.Name("registry-" + strconv.Itoa(port)), Host: "127.0.0.1:" + strconv.Itoa(port), Port: port, cliConfig: filepath.Join(e.Dir, "cli-"+strconv.Itoa(port))}
	args := []string{"run", "-d", "--name", registry.container, "--label", "parachute-e2e=" + e.ID, "--label", "parachute.enable=false",
		"-p", "127.0.0.1:" + strconv.Itoa(port) + ":5000", "--mount", "type=tmpfs,destination=/var/lib/registry"}
	if user != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
		if err != nil {
			e.T.Fatal(err)
		}
		htpasswd := e.WriteFile(fmt.Sprintf("auth-%d/htpasswd", port), user+":"+string(hash)+"\n", 0o644)
		args = append(args, "-v", filepath.Dir(htpasswd)+":/auth", "-e", "REGISTRY_AUTH=htpasswd",
			"-e", "REGISTRY_AUTH_HTPASSWD_REALM=e2e", "-e", "REGISTRY_AUTH_HTPASSWD_PATH=/auth/htpasswd")
	}
	e.Docker(append(args, "registry:2")...)
	if suite.desktop {
		address := e.Docker("inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", registry.container)
		e.Docker("run", "-d", "--network", "host", "--label", "parachute-e2e="+e.ID, "--label", "parachute.enable=false",
			suite.fixtureImage, "forward", registry.Host, address+":5000")
	}
	registry.writeCLIConfig(user, password)
	e.Eventually(30*time.Second, "registry "+registry.Host, func() bool {
		response, err := http.Get("http://" + registry.Host + "/v2/")
		if err != nil {
			return false
		}
		_ = response.Body.Close()
		return response.StatusCode == http.StatusOK || response.StatusCode == http.StatusUnauthorized
	})
	return registry
}

func (r *Registry) writeCLIConfig(user, password string) {
	contents := `{"auths":{}}`
	if user != "" {
		contents = fmt.Sprintf(`{"auths":{%q:{"auth":%q}}}`, r.Host, base64.StdEncoding.EncodeToString([]byte(user+":"+password)))
	}
	if err := os.MkdirAll(r.cliConfig, 0o700); err != nil {
		r.env.T.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.cliConfig, "config.json"), []byte(contents), 0o600); err != nil {
		r.env.T.Fatal(err)
	}
}

func (r *Registry) Ref(repo, tag string) string {
	return r.Host + "/" + r.env.ID + "/" + repo + ":" + tag
}

func (r *Registry) Push(repo, tag, version string) string {
	r.env.T.Helper()
	ref := r.Ref(repo, tag)
	r.env.BuildImage(ref, version)
	r.env.Docker("--config", r.cliConfig, "push", "-q", ref)
	return ref
}

func (r *Registry) Requests() []RegistryRequest {
	r.env.T.Helper()
	logs := r.env.Docker("logs", r.container)
	field := regexp.MustCompile(`http\.request\.(method|uri|useragent)=("(?:[^"\\]|\\.)*"|\S+)`)
	var requests []RegistryRequest
	for _, line := range strings.Split(logs, "\n") {
		if !strings.Contains(line, "response completed") {
			continue
		}
		var request RegistryRequest
		for _, match := range field.FindAllStringSubmatch(line, -1) {
			value := strings.Trim(match[2], `"`)
			switch match[1] {
			case "method":
				request.Method = value
			case "uri":
				request.URI = value
			case "useragent":
				request.UserAgent = value
			}
		}
		requests = append(requests, request)
	}
	return requests
}

func (r *Registry) ParachuteRequests() []RegistryRequest {
	var requests []RegistryRequest
	for _, request := range r.Requests() {
		if strings.HasPrefix(request.UserAgent, "parachute/") {
			requests = append(requests, request)
		}
	}
	return requests
}

func (e *Env) FreePort() int {
	e.T.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		e.T.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		e.T.Fatal(err)
	}
	return port
}
