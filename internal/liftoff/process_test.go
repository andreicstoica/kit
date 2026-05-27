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

func TestParseEtime(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"00:00", 0, true},
		{"01:30", 90, true},
		{"02:03:04", 7384, true},
		{"1-02:03:04", 93784, true},
		{"3-00:00:00", 259200, true},
		{"", 0, false},
		{"garbage", 0, false},
	}
	for _, c := range cases {
		got, ok := parseEtime(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseEtime(%q) = (%d,%v), want (%d,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestLooksStale(t *testing.T) {
	pid := os.Getpid() // a live process started ~now
	// Recorded launch far in the future → live process predates it → stale.
	if !looksStale(pid, time.Now().Add(time.Hour)) {
		t.Errorf("future recorded launch should read as stale")
	}
	// Recorded launch far in the past → not stale.
	if looksStale(pid, time.Now().Add(-time.Hour)) {
		t.Errorf("past recorded launch should not be stale")
	}
	// Unknown recorded time → can't tell → not stale.
	if looksStale(pid, time.Time{}) {
		t.Errorf("zero recorded time should not be stale")
	}
}

// writeCmdStarted writes a minimal .cmd record with the given started: time.
func writeCmdStarted(t *testing.T, worktree, service string, started time.Time) {
	t.Helper()
	if _, err := RunDir(worktree); err != nil {
		t.Fatal(err)
	}
	path, _ := CmdFile(worktree, service)
	body := "started: " + started.UTC().Format(time.RFC3339) + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// startSleeper launches a detached `sleep` as its own group leader (pid==pgid),
// mirroring how StartService spawns services.
func startSleeper(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	go func() { _ = cmd.Wait() }()
	time.Sleep(50 * time.Millisecond)
	t.Cleanup(func() { _ = KillGroup(pid) })
	return pid
}

func TestStopService_StalePIDSkipsKill(t *testing.T) {
	setRunDir(t)
	pid := startSleeper(t)
	if err := WritePID("foo", "app", pid); err != nil {
		t.Fatal(err)
	}
	// Recorded launch in the future → the live process looks recycled → skip kill.
	writeCmdStarted(t, "foo", "app", time.Now().Add(time.Hour))

	if err := StopService("foo", "app"); err != nil {
		t.Fatalf("StopService = %v", err)
	}
	if !IsAlive(pid) {
		t.Errorf("stale guard failed: innocent process was killed")
	}
	if ReadPID("foo", "app") != 0 {
		t.Errorf("stale pid file should have been removed")
	}
}

func TestStopService_KillsLive(t *testing.T) {
	setRunDir(t)
	pid := startSleeper(t)
	if err := WritePID("foo", "app", pid); err != nil {
		t.Fatal(err)
	}
	// Recorded launch ~now matches the process's real start → not stale → kill.
	writeCmdStarted(t, "foo", "app", time.Now())

	if err := StopService("foo", "app"); err != nil {
		t.Fatalf("StopService = %v", err)
	}
	if IsAlive(pid) {
		t.Errorf("live service should have been killed")
	}
	if ReadPID("foo", "app") != 0 {
		t.Errorf("pid file should have been removed")
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
