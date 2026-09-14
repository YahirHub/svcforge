//go:build linux

package svcforge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

var ErrOperationInProgress = errors.New("another lifecycle operation is already in progress")

type installLock struct {
	file *os.File
}

func acquireInstallLock(path string) (*installLock, error) {
	stateDir := filepath.Dir(path)
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("create lifecycle state directory: %w", err)
	}
	if err := os.Chmod(stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("secure lifecycle state directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lifecycle lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrOperationInProgress
		}
		return nil, fmt.Errorf("lock lifecycle state: %w", err)
	}
	return &installLock{file: file}, nil
}

func (l *installLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	if unlockErr != nil {
		return fmt.Errorf("unlock lifecycle state: %w", unlockErr)
	}
	return closeErr
}
