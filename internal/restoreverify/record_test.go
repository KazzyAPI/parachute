package restoreverify

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/internal/osys"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type memFS struct {
	osys.System
	files map[string][]byte
}

func (m *memFS) ReadFile(path string) ([]byte, error) { return m.System.ReadFile(path) }
func (m *memFS) WriteAtomic(path string, data []byte) error {
	if m.files == nil {
		m.files = map[string][]byte{}
	}
	m.files[path] = append([]byte{}, data...)
	return m.System.WriteAtomic(path, data)
}

func (m *memFS) OpenLockFile(path string) (LockFile, error) {
	return osys.New().OpenLockFile(path)
}

func TestRecordOpenAndClose(t *testing.T) {
	dir := t.TempDir()
	files := &memFS{System: *osys.New()}
	runID := "rv-20260201T120000Z-fedcba"
	open := Document{
		Format: Format, ID: runID, Open: true, Started: time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC),
		Stacks: []StackRecord{{Host: "local", Stack: "shop", Snapshot: "snap-20260201T120000Z-abcdef"}},
	}
	if err := WriteRecord(files, dir, open); err != nil {
		t.Fatal(err)
	}
	read, ok, err := ReadRecord(files, dir)
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	if !read.Open || read.ID != runID {
		t.Fatalf("open record %+v", read)
	}
	read.Open = false
	read.Ended = time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	read.Stacks[0].Result = resultPass
	if err := WriteRecord(files, dir, read); err != nil {
		t.Fatal(err)
	}
	closed, ok, err := ReadRecord(files, dir)
	if err != nil || !ok || closed.Open {
		t.Fatalf("closed %+v", closed)
	}
	if closed.Stacks[0].Result != resultPass {
		t.Fatal("result")
	}
	if !txn.ValidRestoreVerifyID(closed.ID) {
		t.Fatal("invalid run id")
	}
	_ = filepath.Join(dir, fileName)
}

func TestLockHeldWhenBusy(t *testing.T) {
	dir := t.TempDir()
	first, err := AcquireLock(osys.New(), dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = AcquireLock(osys.New(), dir)
	if err == nil {
		t.Fatal("expected lock error")
	}
	if !errors.Is(err, ErrLockHeld) {
		t.Fatalf("err %v", err)
	}
	_ = first.Release()
}
