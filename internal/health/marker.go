package health

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
)

type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	WriteAtomic(path string, data []byte) error
	Remove(path string) error
}

type Process interface {
	ProcessID() int
	ProcessAlive(pid int) bool
}

type Marker struct {
	path    string
	files   FileSystem
	process Process
}

func NewMarker(runtimeDir string, files FileSystem, process Process) *Marker {
	return &Marker{path: filepath.Join(runtimeDir, "parachute.ready"), files: files, process: process}
}

func (m *Marker) Ready() error {
	if err := m.files.WriteAtomic(m.path, []byte(strconv.Itoa(m.process.ProcessID()))); err != nil {
		return fmt.Errorf("write readiness marker: %w", err)
	}
	return nil
}

func (m *Marker) Clear() error {
	if err := m.files.Remove(m.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove readiness marker: %w", err)
	}
	return nil
}

func (m *Marker) Healthy() bool {
	contents, err := m.files.ReadFile(m.path)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(contents)))
	return err == nil && pid > 0 && m.process.ProcessAlive(pid)
}
