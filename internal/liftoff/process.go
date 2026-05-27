package liftoff

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// staleTolerance is the slack allowed between a process's kernel start time
// and the launch time kit recorded, to absorb clock skew / NTP steps.
const staleTolerance = 5 * time.Minute

// RunDirPath returns ~/.config/kit/run/<name>/ without touching the filesystem.
// Use for read-only lookups (ReadPID, log paths, status).
func RunDirPath(name string) string {
	if v := os.Getenv("KIT_RUN_DIR"); v != "" {
		return filepath.Join(v, name)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "kit", "run", name)
}

// RunDir returns ~/.config/kit/run/<name>/, creating it if missing.
// Use only when you're about to write (PID file, log file, cmd record).
func RunDir(name string) (string, error) {
	return ensureDir(RunDirPath(name))
}

func ensureDir(p string) (string, error) {
	if err := os.MkdirAll(p, 0o755); err != nil {
		return "", err
	}
	return p, nil
}

// PIDFile returns the service's pid-file path. Does NOT create directories.
func PIDFile(worktree, service string) (string, error) {
	return filepath.Join(RunDirPath(worktree), service+".pid"), nil
}

// LogFile returns the service's log-file path. Does NOT create directories.
func LogFile(worktree, service string) (string, error) {
	return filepath.Join(RunDirPath(worktree), service+".log"), nil
}

// CmdFile returns the service's cmd-record path. Does NOT create directories.
func CmdFile(worktree, service string) (string, error) {
	return filepath.Join(RunDirPath(worktree), service+".cmd"), nil
}

// WritePID writes pid to the pid file. Creates the run dir if missing.
func WritePID(worktree, service string, pid int) error {
	if _, err := RunDir(worktree); err != nil {
		return err
	}
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

// ReadStarted returns the launch time kit recorded in the service's .cmd file
// (the "started:" line), or (zero, false) if absent/unparseable.
func ReadStarted(worktree, service string) (time.Time, bool) {
	path, err := CmdFile(worktree, service)
	if err != nil {
		return time.Time{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		v, ok := strings.CutPrefix(line, "started: ")
		if !ok {
			continue
		}
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(v))
		if err != nil {
			return time.Time{}, false
		}
		return t, true
	}
	return time.Time{}, false
}

// looksStale reports whether pid's live process started before `recorded` —
// i.e. the pid was recycled (e.g. after a reboot). Uses `ps -o etime=` elapsed
// time (portable; macOS lacks etimes). Returns false when it can't tell, so
// pair it with the group-leader guard.
func looksStale(pid int, recorded time.Time) bool {
	if pid <= 0 || recorded.IsZero() {
		return false
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "etime=").Output()
	if err != nil {
		return false
	}
	secs, ok := parseEtime(strings.TrimSpace(string(out)))
	if !ok {
		return false
	}
	startWall := time.Now().Add(-time.Duration(secs) * time.Second)
	return startWall.Before(recorded.Add(-staleTolerance))
}

// parseEtime parses ps elapsed-time format "[[dd-]hh:]mm:ss" into seconds.
func parseEtime(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	days := 0
	if dash := strings.IndexByte(s, '-'); dash >= 0 {
		d, err := strconv.Atoi(s[:dash])
		if err != nil {
			return 0, false
		}
		days = d
		s = s[dash+1:]
	}
	parts := strings.Split(s, ":")
	var h, m, sec int
	switch len(parts) {
	case 2:
		m = atoiField(parts[0])
		sec = atoiField(parts[1])
	case 3:
		h = atoiField(parts[0])
		m = atoiField(parts[1])
		sec = atoiField(parts[2])
	default:
		return 0, false
	}
	return days*86400 + h*3600 + m*60 + sec, true
}

func atoiField(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
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

// SweepOldRunDirs removes ~/.config/kit/run/<name>/ subdirs whose most-recent
// file mtime is older than maxAge AND which have no live PID. Safe to call
// passively — returns the count of dirs removed and a list of errors that
// occurred (so the caller can log but not fail). 0/nil on a clean sweep.
func SweepOldRunDirs(maxAge time.Duration) (int, []error) {
	base := RunDirPath("")
	base = filepath.Clean(base) // strip trailing "/"
	entries, err := os.ReadDir(base)
	if err != nil {
		return 0, nil
	}
	cutoff := time.Now().Add(-maxAge)
	var errs []error
	removed := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		dir := filepath.Join(base, name)

		// Don't sweep dirs that still own live PID files.
		if hasLivePID(dir) {
			continue
		}

		newest, ok := newestFileMtime(dir)
		if !ok || newest.After(cutoff) {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", dir, err))
			continue
		}
		removed++
	}
	return removed, errs
}

func newestFileMtime(dir string) (time.Time, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return time.Time{}, false
	}
	var newest time.Time
	any := false
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		any = true
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return newest, any
}

func hasLivePID(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".pid") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			continue
		}
		if IsAlive(pid) {
			return true
		}
	}
	return false
}
