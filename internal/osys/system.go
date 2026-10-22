package osys

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

type System struct{}

func New() *System { return &System{} }

func (System) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (System) Stat(path string) (fs.FileInfo, error) { return os.Stat(path) }

func (System) Hostname() (string, error) { return os.Hostname() }

func (System) TempDir() string { return os.TempDir() }

func (System) Remove(path string) error { return os.Remove(path) }

func (s System) WriteAtomic(path string, data []byte) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("create temporary file in %s: %w", directory, err)
	}
	cleanup := func(cause error) error {
		return fmt.Errorf("write %s: %w", path, errors.Join(cause, temporary.Close(), os.Remove(temporary.Name())))
	}
	if _, err := temporary.Write(data); err != nil {
		return cleanup(err)
	}
	if err := temporary.Sync(); err != nil {
		return cleanup(err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, errors.Join(err, os.Remove(temporary.Name())))
	}
	if err := os.Rename(temporary.Name(), path); err != nil {
		return fmt.Errorf("write %s: %w", path, errors.Join(err, os.Remove(temporary.Name())))
	}
	return s.syncDirectory(directory)
}

func (System) syncDirectory(directory string) error {
	handle, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open %s: %w", directory, err)
	}
	syncErr := handle.Sync()
	closeErr := handle.Close()
	if syncErr != nil || closeErr != nil {
		return fmt.Errorf("sync %s: %w", directory, errors.Join(syncErr, closeErr))
	}
	return nil
}

func (System) ProcessID() int { return os.Getpid() }

func (System) ProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}
