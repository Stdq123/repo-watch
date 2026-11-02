//go:build darwin

package main

import (
	"os"
	"path/filepath"
	"syscall"
)

// The file stays in place across launches: unlinking a locked file would let
// a second process create a different inode and bypass the lock.
func acquireMacInstanceLock(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}
