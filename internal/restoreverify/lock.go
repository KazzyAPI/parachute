package restoreverify

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/KazzyAPI/parachute/internal/osys"
	"github.com/KazzyAPI/parachute/internal/txn"
)

const lockName = "restore-verify.lock"

var ErrLockHeld = errors.New("restore-verify.lock is held by another process")

type Locker struct {
	files FileSystem
	dir   string
	lock  LockFile
}

func AcquireLock(files FileSystem, dir string) (*Locker, error) {
	path := filepath.Join(dir, lockName)
	lock, err := files.OpenLockFile(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", lockName, err)
	}
	if err := lock.TryLockExclusive(); errors.Is(err, osys.ErrLocked) {
		_ = lock.Unlock()
		return nil, ErrLockHeld
	} else if err != nil {
		_ = lock.Unlock()
		return nil, fmt.Errorf("lock %s: %w", lockName, err)
	}
	return &Locker{files: files, dir: dir, lock: lock}, nil
}

func (l *Locker) Release() error {
	if l.lock == nil {
		return nil
	}
	err := l.lock.Unlock()
	l.lock = nil
	return err
}

func LockFileName() string { return lockName }

func AllowedRunIDs(document Document, current string) []string {
	var ids []string
	if current != "" && txn.ValidRestoreVerifyID(current) {
		ids = append(ids, current)
	}
	if open := OpenRunID(document); open != "" && open != current {
		ids = append(ids, open)
	}
	return ids
}
