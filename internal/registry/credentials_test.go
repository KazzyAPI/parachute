package registry

import (
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
