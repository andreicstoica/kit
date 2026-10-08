package liftoff

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// The config lock protects individual writes, not an external create-then-save
// operation. Serialize mutations per workspace so two opens cannot create two
// serverside sessions and lose one of their ownership IDs. Other worktrees are
// independent and do not wait on this lock.
func withWorkspaceLock(key string, run func() error) error {
	return WithWorkspaceLock(key, run)
}

// WithWorkspaceLock serializes operations that share external workspace resources.
func WithWorkspaceLock(key string, run func() error) error {
	dir := filepath.Join(configDir(), "workspace-locks")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, fmt.Sprintf("%x.lock", sha256.Sum256([]byte(key))))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	deadline := time.Now().Add(15 * time.Second)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("workspace %q is busy; retry after the other operation completes", key)
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return run()
}
