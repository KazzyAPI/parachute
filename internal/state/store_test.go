package state

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
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
	store := NewStore("/s", files, slog.New(slog.NewTextHandler(io.Discard, nil)))
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
	reopened := NewStore("/s", files, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := reopened.Open(); err != nil {
		t.Fatal(err)
	}
	if current := reopened.Current(); len(current.SupersededInstances) != 1 || current.LastCycle.ID != "cyc" {
		t.Errorf("unexpected document %+v", current)
	}
}

func TestFailedUpdateLeavesCurrentUntouched(t *testing.T) {
	store := NewStore("/s", memoryFiles{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	drift := "none"
	if err := NewRecorder(store).Publish(context.Background(), domain.Report{Hosts: []domain.HostReport{{Results: []domain.Result{{Drift: &drift}}}}}); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("refused")
	err := store.Update(func(document *Document) error {
		*document.Results[0].Drift = "changed"
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("Update returned %v", err)
	}
	if got := *store.Current().Results[0].Drift; got != "none" {
		t.Errorf("current drift %q after a failed update", got)
	}
}

func TestUnknownFieldsAreRejected(t *testing.T) {
	files := memoryFiles{"/s/state.json": []byte(`{"format":"parachute.state/v1","armed":true}`)}
	if err := NewStore("/s", files, slog.New(slog.NewTextHandler(io.Discard, nil))).Open(); err == nil || !strings.Contains(err.Error(), "/s/state.json") {
		t.Fatalf("Open returned %v", err)
	}
}

func TestMissingStateDirectoryNamesTheVolumeToMount(t *testing.T) {
	err := fmt.Errorf("write: %w", fs.ErrNotExist)
	if hint := mountHint(err, "/var/lib/parachute"); !strings.Contains(hint, "-v parachute-state:/var/lib/parachute") {
		t.Errorf("hint %q", hint)
	}
	if hint := mountHint(errors.New("disk full"), "/x"); hint != "" {
		t.Errorf("hint %q", hint)
	}
}
