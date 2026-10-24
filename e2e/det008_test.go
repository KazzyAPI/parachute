package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func headDigest(value string) harness.ProxyOptions {
	return harness.ProxyOptions{Rewrite: rewriteManifest(http.MethodHead, func(header http.Header, body []byte) []byte {
		if value == "" {
			header.Del("Docker-Content-Digest")
		} else {
			header.Set("Docker-Content-Digest", value)
		}
		return body
	})}
}

func TestDET_008_pull_is_by_candidate_digest_never_by_tag(t *testing.T) {
	env := harness.NewEnv(t)
	app := newUpdatable(env, "app")
	pushed := make(chan struct{})
	var swap sync.Once
	proxy := app.registry.Proxy(harness.ProxyOptions{Handle: func(_ http.ResponseWriter, request *http.Request) bool {
		if request.Method == http.MethodHead && strings.HasSuffix(request.URL.Path, "/manifests/1") {
			swap.Do(func() {
				go func() {
					defer close(pushed)
					app.registry.Push("app", "1", "3")
				}()
			})
		}
		return false
	}})
	a := env.RunContainer("a", env.Rehome(app.ref, proxy.Host), harness.ContainerOptions{})
	app.publishNewer()
	result := armedParachute(env, "--backup", "none").Run(runTimeout)
	<-pushed
	entry := assertState(t, result, a, "updated", "")
	for _, request := range manifestGets(proxy) {
		if strings.HasSuffix(request.Path, "/manifests/1") {
			t.Errorf("manifest fetched by tag: %v", request)
		}
	}
	_, image := env.ContainerState(a)
	if digests := env.Docker("image", "inspect", "--format", "{{json .RepoDigests}}", image); !strings.Contains(digests, entry["candidate_digest"].(string)) {
		t.Errorf("replacement image digests %s do not contain candidate %v", digests, entry["candidate_digest"])
	}
}

func TestDET_008_invalid_head_digest_fails_without_get(t *testing.T) {
	env := harness.NewEnv(t)
	a, proxy := proxiedUpdatable(env, headDigest("sha256:abc"))
	assertState(t, runOnce(env), a, "failed", "registry_unreachable")
	if gets := manifestGets(proxy); len(gets) > 0 {
		t.Errorf("manifest GET after a 200 HEAD: %v", gets)
	}
}

func TestDET_008_missing_head_digest_fails_without_get(t *testing.T) {
	env := harness.NewEnv(t)
	a, proxy := proxiedUpdatable(env, headDigest(""))
	assertState(t, runOnce(env), a, "failed", "registry_unreachable")
	if gets := manifestGets(proxy); len(gets) > 0 {
		t.Errorf("manifest GET after a 200 HEAD: %v", gets)
	}
}

func TestDET_008_get_header_mismatch_fails(t *testing.T) {
	env := harness.NewEnv(t)
	a, _ := proxiedUpdatable(env, harness.ProxyOptions{RejectHead: true, Rewrite: rewriteManifest(http.MethodGet, func(header http.Header, body []byte) []byte {
		header.Set("Docker-Content-Digest", "sha256:"+strings.Repeat("0", 64))
		return body
	})})
	assertState(t, runOnce(env), a, "failed", "registry_unreachable")
}

func TestDET_008_get_without_header_uses_body_digest(t *testing.T) {
	env := harness.NewEnv(t)
	served := make(chan string, 1)
	a, _ := proxiedUpdatable(env, harness.ProxyOptions{RejectHead: true, Rewrite: rewriteManifest(http.MethodGet, func(header http.Header, body []byte) []byte {
		sum := sha256.Sum256(body)
		select {
		case served <- "sha256:" + hex.EncodeToString(sum[:]):
		default:
		}
		header.Del("Docker-Content-Digest")
		return body
	})})
	entry := assertState(t, runOnce(env), a, "would_update", "")
	select {
	case want := <-served:
		if entry["candidate_digest"] != want {
			t.Errorf("candidate digest %v, want %s", entry["candidate_digest"], want)
		}
	default:
		t.Fatal("proxy served no manifest by GET")
	}
}
