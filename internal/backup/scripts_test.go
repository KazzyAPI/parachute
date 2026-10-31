package backup

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runScript(t *testing.T, script, root string, args ...string) error {
	t.Helper()
	script = strings.NewReplacer("/src", root+"/src", "/snap", root+"/snap", "/target", root+"/target").Replace(script)
	command := exec.Command("sh", append([]string{"-c", script, "sh"}, args...)...)
	return command.Run()
}

func TestCopyScriptFailsWhenTarCannotReadAFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads unreadable files")
	}
	root := t.TempDir()
	for _, dir := range []string{"src", "snap/s"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "src", "a"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "b"), []byte("b"), 0o000); err != nil {
		t.Fatal(err)
	}
	if err := runScript(t, copyScript, root, "s", "f.tar.gz"); err == nil {
		t.Fatal("tar failure must fail the copy")
	}
	if _, err := os.Stat(filepath.Join(root, "snap", "s", "f.tar.gz")); err == nil {
		t.Fatal("no archive may be published after a tar failure")
	}
}

func TestExtractScriptRemovesFilesAbsentFromSnapshot(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"src", "snap/s", "target"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, text string) {
		if err := os.WriteFile(filepath.Join(root, path), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("src/version", "old")
	if err := runScript(t, copyScript, root, "s", "f.tar.gz"); err != nil {
		t.Fatal(err)
	}
	write("target/version", "new")
	write("target/new.db", "x")
	write("target/.hidden", "x")
	if err := runScript(t, extractScript, root, "s", "f.tar.gz"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "target"))
	if err != nil || len(entries) != 1 || entries[0].Name() != "version" {
		t.Fatalf("entries %v err %v", entries, err)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "target", "version")); string(data) != "old" {
		t.Fatalf("version %q", data)
	}
}
