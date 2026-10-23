package harness

import (
	"context"
	"fmt"
	"maps"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

func (e *Env) BuildParachuteImage(ref, version string, labels map[string]string) {
	e.T.Helper()
	context, err := os.MkdirTemp(e.Dir, "parachute-build-")
	if err != nil {
		e.T.Fatal(err)
	}
	var file strings.Builder
	fmt.Fprintf(&file, "FROM %s AS source\nFROM scratch\nCOPY --from=source / /\nLABEL parachute-e2e.version=%s-%s\n", e.ParachuteImage(), e.ID, version)
	for _, key := range slices.Sorted(maps.Keys(labels)) {
		fmt.Fprintf(&file, "LABEL %q=%q\n", key, labels[key])
	}
	file.WriteString("ENTRYPOINT [\"parachute\"]\n")
	if err := os.WriteFile(filepath.Join(context, "Dockerfile"), []byte(file.String()), 0o644); err != nil {
		e.T.Fatal(err)
	}
	e.Docker("build", "-q", "--provenance=false", "-t", ref, context)
	e.trackImage(ref)
}

func (e *Env) CreateParachuteContainer(name, image string, extra []string, run ContainerRun) string {
	e.T.Helper()
	args := e.containerArgs(name, run)
	index := slices.Index(args, e.ParachuteImage())
	args = slices.Concat([]string{"create"}, args[1:index], extra, []string{image}, args[index+1:])
	e.Docker(args...)
	return e.Name(name)
}

func (e *Env) StartAttached(container string, timeout time.Duration) *Result {
	e.T.Helper()
	return e.runCommand(timeout, nil, "docker", "start", "-a", container)
}

type ImageFaultProxy struct {
	socket   string
	failing  []string
	lock     sync.Mutex
	inspects []string
}

func (e *Env) ImageFaultProxy(failing ...string) *ImageFaultProxy {
	e.T.Helper()
	dir, err := os.MkdirTemp("", "pe-engine-")
	if err != nil {
		e.T.Fatal(err)
	}
	proxy := &ImageFaultProxy{socket: filepath.Join(dir, "docker.sock"), failing: failing}
	listener, err := net.Listen("unix", proxy.socket)
	if err != nil {
		e.T.Fatal(err)
	}
	engine := strings.TrimPrefix(suite.dockerHost, "unix://")
	forward := httputil.NewSingleHostReverseProxy(&url.URL{Scheme: "http", Host: "docker"})
	forward.Transport = &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", engine)
	}}
	server := &http.Server{Handler: http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if proxy.failImageInspect(request) {
			http.Error(response, `{"message":"injected image inspect failure"}`, http.StatusInternalServerError)
			return
		}
		forward.ServeHTTP(response, request)
	}), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = server.Serve(listener) }()
	e.T.Cleanup(func() {
		_ = server.Close()
		_ = os.RemoveAll(dir)
	})
	return proxy
}

func (p *ImageFaultProxy) Endpoint() string { return "unix://" + p.socket }

func (p *ImageFaultProxy) failImageInspect(request *http.Request) bool {
	path, err := url.PathUnescape(request.URL.EscapedPath())
	if err != nil || request.Method != http.MethodGet || !regexp.MustCompile(`/images/.+/json$`).MatchString(path) {
		return false
	}
	p.lock.Lock()
	p.inspects = append(p.inspects, path)
	p.lock.Unlock()
	return slices.ContainsFunc(p.failing, func(image string) bool { return strings.Contains(path, image) })
}

func (p *ImageFaultProxy) ImageInspects() []string {
	p.lock.Lock()
	defer p.lock.Unlock()
	return slices.Clone(p.inspects)
}
