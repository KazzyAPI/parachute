package harness

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type Env struct {
	T         *testing.T
	ID        string
	Dir       string
	StateDir  string
	ConfigDir string
	lock      sync.Mutex
	images    []string

	registryOnce sync.Once
	registry     *Registry
}

func NewEnv(t *testing.T) *Env {
	t.Helper()
	t.Parallel()
	dir := t.TempDir()
	env := &Env{T: t, ID: "pe" + randomID(), Dir: dir, StateDir: filepath.Join(dir, "state"), ConfigDir: filepath.Join(dir, "docker-config")}
	for _, path := range []string{env.StateDir, env.ConfigDir} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(env.cleanup)
	return env
}

func randomID() string {
	return strings.ToLower(rand.Text()[:8])
}

func (e *Env) Name(suffix string) string { return e.ID + "-" + suffix }

func (e *Env) Scope() string { return e.ID }

func (e *Env) Docker(args ...string) string {
	e.T.Helper()
	out, err := suite.command("", nil, "docker", args...)
	if err != nil {
		e.T.Fatalf("docker %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(out)
}

func (e *Env) DockerMayFail(args ...string) (string, error) {
	out, err := suite.command("", nil, "docker", args...)
	return strings.TrimSpace(out), err
}

func (e *Env) trackImage(ref string) {
	e.lock.Lock()
	defer e.lock.Unlock()
	e.images = append(e.images, ref)
}

func (e *Env) cleanup() {
	containers, err := e.DockerMayFail("ps", "-aq", "--filter", "label=parachute-e2e="+e.ID)
	if err != nil {
		e.T.Logf("cleanup: list containers: %v", err)
	}
	if ids := strings.Fields(containers); len(ids) > 0 {
		if out, err := e.DockerMayFail(append([]string{"rm", "-f", "-v"}, ids...)...); err != nil {
			e.T.Logf("cleanup: remove containers: %v %s", err, out)
		}
	}
	if out, err := e.DockerMayFail("run", "--rm", "--label", "parachute-e2e="+e.ID, "--entrypoint", "chmod", "-v", e.Dir+":"+e.Dir, suite.FixtureV2Image(), "-R", "a+rwX", e.Dir); err != nil {
		e.T.Logf("cleanup: open test directory: %v %s", err, out)
	}
	networks, err := e.DockerMayFail("network", "ls", "-q", "--filter", "name="+e.ID)
	if err != nil {
		e.T.Logf("cleanup: list networks: %v", err)
	}
	for _, network := range strings.Fields(networks) {
		if out, err := e.DockerMayFail("network", "rm", network); err != nil {
			e.T.Logf("cleanup: remove network: %v %s", err, out)
		}
	}
	e.lock.Lock()
	images := append([]string{}, e.images...)
	e.lock.Unlock()
	if len(images) > 0 {
		if out, err := e.DockerMayFail(append([]string{"rmi", "-f"}, images...)...); err != nil {
			e.T.Logf("cleanup: remove images: %v %s", err, out)
		}
	}
}

func (e *Env) WriteFile(name, contents string, mode os.FileMode) string {
	e.T.Helper()
	path := filepath.Join(e.Dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		e.T.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		e.T.Fatal(err)
	}
	return path
}

func (e *Env) Eventually(timeout time.Duration, what string, condition func() bool) {
	e.T.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	e.T.Fatalf("timed out after %s waiting for %s", timeout, what)
}
