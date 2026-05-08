package liftoff

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func setRunDir(t *testing.T) {
	t.Helper()
	t.Setenv("KIT_RUN_DIR", t.TempDir())
}

func TestPIDFileLifecycle(t *testing.T) {
	setRunDir(t)
	if err := WritePID("foo", "app", 12345); err != nil {
		t.Fatal(err)
	}
	if got := ReadPID("foo", "app"); got != 12345 {
		t.Errorf("ReadPID = %d, want 12345", got)
	}
	if err := RemovePID("foo", "app"); err != nil {
		t.Fatal(err)
	}
	if got := ReadPID("foo", "app"); got != 0 {
		t.Errorf("after remove, ReadPID = %d, want 0", got)
	}
}

func TestRemovePID_Missing(t *testing.T) {
	setRunDir(t)
	if err := RemovePID("foo", "app"); err != nil {
		t.Errorf("RemovePID(missing) = %v, want nil", err)
	}
}

func TestIsAlive_Self(t *testing.T) {
	if !IsAlive(os.Getpid()) {
		t.Errorf("self should be alive")
	}
}

func TestIsAlive_Dead(t *testing.T) {
	if IsAlive(0) {
		t.Errorf("pid 0 should not be alive")
	}
	// Pick a high PID unlikely to exist.
	if IsAlive(999999) {
		t.Errorf("pid 999999 should not be alive")
	}
}

func TestKillGroup(t *testing.T) {
	cmd := exec.Command("sleep", "5")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	go func() { _ = cmd.Wait() }()
	time.Sleep(50 * time.Millisecond)
	if !IsAlive(pid) {
		t.Fatal("sleep didn't start")
	}
	if err := KillGroup(pid); err != nil {
		t.Fatal(err)
	}
	if IsAlive(pid) {
		t.Errorf("sleep still alive after KillGroup")
	}
}

func TestSweepStalePID(t *testing.T) {
	setRunDir(t)
	_ = WritePID("foo", "app", 999998) // dead pid
	SweepStalePID("foo", "app")
	path, _ := PIDFile("foo", "app")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("stale pid file not swept: %v", err)
	}
}

func TestRunDir_EnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KIT_RUN_DIR", dir)
	got, err := RunDir("voice-agent")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "voice-agent")
	if got != want {
		t.Errorf("RunDir = %q, want %q", got, want)
	}
	if _, err := os.Stat(got); err != nil {
		t.Errorf("RunDir not created: %v", err)
	}
}
