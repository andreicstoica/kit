package liftoff

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// RunDir returns ~/.config/kit/run/<name>/.
// Creates the dir if missing.
func RunDir(name string) (string, error) {
	if v := os.Getenv("KIT_RUN_DIR"); v != "" {
		return ensureDir(filepath.Join(v, name))
	}
	home, _ := os.UserHomeDir()
	return ensureDir(filepath.Join(home, ".config", "kit", "run", name))
}

func ensureDir(p string) (string, error) {
	if err := os.MkdirAll(p, 0o755); err != nil {
		return "", err
	}
	return p, nil
}

// PIDFile is the path for a service's pid file.
func PIDFile(worktree, service string) (string, error) {
	dir, err := RunDir(worktree)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, service+".pid"), nil
}

// LogFile is the path for a service's combined log.
func LogFile(worktree, service string) (string, error) {
	dir, err := RunDir(worktree)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, service+".log"), nil
}

// CmdFile is the path for the recorded command line + env.
func CmdFile(worktree, service string) (string, error) {
	dir, err := RunDir(worktree)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, service+".cmd"), nil
}

// WritePID writes pid to the pid file.
func WritePID(worktree, service string, pid int) error {
	path, err := PIDFile(worktree, service)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0o644)
}

// ReadPID returns the PID from the file, or 0 if absent or stale.
func ReadPID(worktree, service string) int {
	path, err := PIDFile(worktree, service)
	if err != nil {
		return 0
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return pid
}

// RemovePID deletes the pid file. Idempotent.
func RemovePID(worktree, service string) error {
	path, err := PIDFile(worktree, service)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// IsAlive returns true if a process with this PID is running.
// Uses signal 0 (no-op) to probe.
func IsAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil
}

// KillGroup sends SIGTERM to the process group, waits up to 3 seconds for
// the leader to die, then escalates to SIGKILL on the group.
func KillGroup(pid int) error {
	if pid <= 0 {
		return nil
	}
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		// Fallback: signal the pid directly.
		pgid = pid
	}
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	// Polite wait.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !IsAlive(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	// Escalate.
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
	for i := 0; i < 10; i++ {
		if !IsAlive(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("pid %d still alive after SIGKILL", pid)
}

// SweepStalePID removes the pid file if its process is gone.
func SweepStalePID(worktree, service string) {
	pid := ReadPID(worktree, service)
	if pid == 0 {
		return
	}
	if !IsAlive(pid) {
		_ = RemovePID(worktree, service)
	}
}
