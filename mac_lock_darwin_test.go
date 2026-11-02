//go:build darwin

package main

import (
	"path/filepath"
	"testing"
)

func TestMacInstanceLockExcludesAnotherProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instance.lock")
	first, err := acquireMacInstanceLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := acquireMacInstanceLock(path); err == nil {
		second.Close()
		t.Fatal("second instance obtained the lock")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	third, err := acquireMacInstanceLock(path)
	if err != nil {
		t.Fatalf("lock was not released: %v", err)
	}
	third.Close()
}
