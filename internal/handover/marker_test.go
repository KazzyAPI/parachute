package handover

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

type memoryFiles map[string][]byte

func (m memoryFiles) ReadFile(path string) ([]byte, error) {
	contents, ok := m[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return contents, nil
}

func (m memoryFiles) WriteAtomic(path string, data []byte) error {
	m[path] = data
	return nil
}

func (m memoryFiles) Remove(path string) error {
	delete(m, path)
	return nil
}

func validMarker() Marker {
	return Marker{
		Format:      Format,
		Transaction: "txn-20260101T000000Z-abc123",
		Container:   strings.Repeat("a", 64),
		Digest:      "sha256:deadbeef",
	}
}

func TestWriteReadRemoveRoundTrip(t *testing.T) {
	files := memoryFiles{}
	dir := "/state"
	marker := validMarker()
	if err := Write(files, dir, marker); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, fileName)
	if len(files[path]) == 0 {
		t.Fatal("marker not written")
	}
	got, ok, err := Read(files, dir)
	if err != nil || !ok {
		t.Fatalf("read: ok=%t err=%v", ok, err)
	}
	if got != marker {
		t.Fatalf("got %+v want %+v", got, marker)
	}
	if err := Remove(files, dir); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := Read(files, dir); err != nil || ok {
		t.Fatalf("after remove: ok=%t err=%v", ok, err)
	}
}

func TestReadRejectsUnknownFields(t *testing.T) {
	files := memoryFiles{filepath.Join("/s", fileName): []byte(`{"format":"parachute.handover/v1","transaction":"txn-20260101T000000Z-abc123","container":"` + strings.Repeat("b", 64) + `","digest":"sha256:x","extra":1}`)}
	_, ok, err := Read(files, "/s")
	if err == nil || !ok {
		t.Fatalf("ok=%t err=%v", ok, err)
	}
	if !strings.Contains(err.Error(), "extra") {
		t.Fatalf("error %q", err)
	}
}

func TestReadRejectsBadTransactionID(t *testing.T) {
	marker := validMarker()
	marker.Transaction = "bad"
	files := memoryFiles{}
	if err := Write(files, "/s", marker); err == nil {
		t.Fatal("write should fail")
	}
	files[filepath.Join("/s", fileName)] = []byte(`{"format":"parachute.handover/v1","transaction":"bad","container":"` + strings.Repeat("c", 64) + `","digest":"sha256:x"}`)
	_, ok, err := Read(files, "/s")
	if err == nil || !ok {
		t.Fatalf("ok=%t err=%v", ok, err)
	}
	if !strings.Contains(err.Error(), "transaction") {
		t.Fatalf("error %q", err)
	}
}

func TestReadRejectsBadContainerID(t *testing.T) {
	files := memoryFiles{filepath.Join("/s", fileName): []byte(`{"format":"parachute.handover/v1","transaction":"txn-20260101T000000Z-abc123","container":"short","digest":"sha256:x"}`)}
	_, ok, err := Read(files, "/s")
	if err == nil || !ok {
		t.Fatalf("ok=%t err=%v", ok, err)
	}
	if !strings.Contains(err.Error(), "container") {
		t.Fatalf("error %q", err)
	}
}
