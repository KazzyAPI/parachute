package state

import (
	"context"
	"io/fs"
	"strings"
	"testing"

	"github.com/KazzyAPI/parachute/internal/domain"
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

func TestRecorderKeepsFieldsItDoesNotOwn(t *testing.T) {
	files := memoryFiles{}
	store := NewStore("/s", files)
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(func(document *Document) error {
		document.SupersededInstances = append(document.SupersededInstances, "abc")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := NewRecorder(store).Publish(context.Background(), domain.Report{CycleID: "cyc"}); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore("/s", files)
	if err := reopened.Open(); err != nil {
		t.Fatal(err)
	}
	if current := reopened.Current(); len(current.SupersededInstances) != 1 || current.LastCycle.ID != "cyc" {
		t.Errorf("unexpected document %+v", current)
	}
}

func TestUnknownFieldsAreRejected(t *testing.T) {
	files := memoryFiles{"/s/state.json": []byte(`{"format":"parachute.state/v1","armed":true}`)}
	if err := NewStore("/s", files).Open(); err == nil || !strings.Contains(err.Error(), "/s/state.json") {
		t.Fatalf("Open returned %v", err)
	}
}
