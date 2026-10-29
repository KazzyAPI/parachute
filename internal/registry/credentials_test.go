package registry

import (
	"encoding/base64"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

type diskFiles struct{}

func (diskFiles) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (diskFiles) Stat(path string) (fs.FileInfo, error) { return os.Stat(path) }

func TestSecretsFollowConfigChangesAndKeepLastGoodOnParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	store := NewCredentialStore(dir, diskFiles{})
	write := func(contents string, modified time.Time) {
		t.Helper()
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Unix(1_700_000_000, 0)
	write(`{"auths":{"r":{"username":"u","password":"first"}}}`, start)
	if got := store.Secrets(); !slices.Contains(got, "first") {
		t.Fatalf("secrets %v", got)
	}
	write(`{"auths":{"r":{"username":"u","password":"second"}}}`, start.Add(time.Second))
	if got := store.Secrets(); !slices.Contains(got, "second") || slices.Contains(got, "first") {
		t.Fatalf("secrets after change %v", got)
	}
	write(`not json`, start.Add(2*time.Second))
	if got := store.Secrets(); !slices.Contains(got, "second") {
		t.Fatalf("secrets after parse error %v", got)
	}
}

func TestPullAuthEncodesStoredCredential(t *testing.T) {
	credentials := Credentials{byHost: map[string]Credential{"127.0.0.1:5000": {Username: "user", Password: "pw"}}, helpers: map[string]string{}}
	ref := Reference{Domain: "127.0.0.1:5000", Repository: "app"}
	raw, err := base64.URLEncoding.DecodeString(credentials.PullAuth(ref))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"password":"pw","serveraddress":"127.0.0.1:5000","username":"user"}`
	if string(raw) != want {
		t.Fatalf("got %s", raw)
	}
	if credentials.PullAuth(Reference{Domain: "other", Repository: "app"}) != "" {
		t.Fatal("expected empty auth for unknown registry")
	}
}
