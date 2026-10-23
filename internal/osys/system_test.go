package osys

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestReadRegularFileRejectsSpecialAndLargeFiles(t *testing.T) {
	dir := t.TempDir()
	regular, large, fifo, link := filepath.Join(dir, "a.yml"), filepath.Join(dir, "big"), filepath.Join(dir, "fifo"), filepath.Join(dir, "link")
	if err := os.WriteFile(regular, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(large, []byte(strings.Repeat("x", 65)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(regular, link); err != nil {
		t.Fatal(err)
	}
	system := New()
	if contents, err := system.ReadRegularFile(regular, 64); err != nil || string(contents) != "services: {}\n" {
		t.Errorf("regular file: %q, %v", contents, err)
	}
	for _, path := range []string{large, fifo, link, "/dev/zero", dir, filepath.Join(dir, "missing")} {
		if _, err := system.ReadRegularFile(path, 64); err == nil {
			t.Errorf("%s was read", path)
		}
	}
}
