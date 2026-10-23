package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
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
