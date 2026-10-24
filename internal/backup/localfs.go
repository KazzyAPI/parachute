package backup

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
)

type LocalFS struct{}

func (LocalFS) Resolve(_ context.Context, path string) (string, error) {
	return filepath.EvalSymlinks(path)
}

func (LocalFS) Size(_ context.Context, path string) (int64, error) {
	return dirSize(path)
}

func (LocalFS) FreeBytes(_ context.Context, path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}

func dirSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	if !info.IsDir() {
		return info.Size(), nil
	}
	var total int64
	err = filepath.WalkDir(path, func(_ string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return err
		}
		total += fileInfo.Size()
		return nil
	})
	return total, err
}
