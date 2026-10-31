package osys

import (
	"errors"
	"os"
	"syscall"
)

var ErrLocked = errors.New("file is locked")

type LockFile interface {
	TryLockExclusive() error
	Unlock() error
}

type FileLock struct {
	file *os.File
}

func (System) OpenLockFile(path string) (LockFile, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	return &FileLock{file: file}, nil
}

func (l *FileLock) TryLockExclusive() error {
	err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return ErrLocked
	}
	return err
}

func (l *FileLock) Unlock() error {
	unlockErr := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	return errors.Join(unlockErr, closeErr)
}
