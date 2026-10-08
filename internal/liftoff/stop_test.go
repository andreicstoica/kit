package liftoff

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestLivePIDsOnlyQueriesTargets(t *testing.T) {
	bin := t.TempDir()
	capture := filepath.Join(t.TempDir(), "ps-args")
	t.Setenv("PS_CAPTURE", capture)
	writeExecutable(t, filepath.Join(bin, "ps"), "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$PS_CAPTURE\"\nexec /bin/ps \"$@\"\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	pid := os.Getpid()
	got := livePIDs([]int{pid, 99999999})
	if !slices.Equal(got, []int{pid}) {
		t.Fatalf("live pids = %v", got)
	}
	args := readCapture(t, capture)
	if !strings.Contains(args, "-p ") || strings.Contains(args, "-axo") {
		t.Fatalf("shutdown polling scans unrelated processes: %s", args)
	}
}

func BenchmarkLivePIDs(b *testing.B) {
	pids := []int{os.Getpid()}
	for i := 0; i < b.N; i++ {
		livePIDs(pids)
	}
}

func TestParsePS(t *testing.T) {
	out := "  100     1   100 Ss\n  101   100   100 S+\n 102 101 102 Z\nbad line\n"
	got := parsePS(out)
	if len(got) != 3 {
		t.Fatalf("parsePS = %+v", got)
	}
	if p := got[101]; p.PPID != 100 || p.PGID != 100 || p.Zombie {
		t.Errorf("101 = %+v", p)
	}
	if !got[102].Zombie {
		t.Errorf("102 should be a zombie")
	}
}

func TestParseTagged(t *testing.T) {
	out := strings.Join([]string{
		"100 python celery worker PATH=/bin KIT_SERVICE=wt/celery HOME=/h",
		"101 python celery worker KIT_SERVICE=wt/celery",
		"102 python celery worker KIT_SERVICE=wt/celery2",
		"103 python KIT_SERVICE=other/celery",
		"104 grep xKIT_SERVICE=wt/celery",
	}, "\n")
	got := parseTagged(out, "KIT_SERVICE=wt/celery")
	if !slices.Equal(got, []int{100, 101}) {
		t.Errorf("parseTagged = %v, want [100 101]", got)
	}
}

func TestServiceTree(t *testing.T) {
	procs := map[int]procInfo{
		1:   {PID: 1, PPID: 0, PGID: 1},
		50:  {PID: 50, PPID: 1, PGID: 50},     // self (kit)
		100: {PID: 100, PPID: 1, PGID: 100},   // recorded worker
		101: {PID: 101, PPID: 100, PGID: 100}, // prefork child
		102: {PID: 102, PPID: 101, PGID: 102}, // grandchild in its own group
		103: {PID: 103, PPID: 1, PGID: 100},   // group member orphaned to launchd
		104: {PID: 104, PPID: 100, PGID: 100, Zombie: true},
		200: {PID: 200, PPID: 1, PGID: 200},               // unrelated
		201: {PID: 201, PPID: 200, PGID: 200},             // unrelated child
		300: {PID: 300, PPID: 1, PGID: 300, Tagged: true}, // untracked old worker
		301: {PID: 301, PPID: 300, PGID: 300},
		400: {PID: 400, PPID: 1, PGID: 200, Tagged: true}, // tagged but not a leader
	}
	pids, groups := serviceTree(procs, 100, 50)
	if want := []int{100, 101, 102, 103, 300, 301, 400}; !slices.Equal(pids, want) {
		t.Errorf("pids = %v, want %v", pids, want)
	}
	// 200's group is not ours even though 400 sits in it.
	if want := []int{100, 102, 300}; !slices.Equal(groups, want) {
		t.Errorf("groups = %v, want %v", groups, want)
	}

	if pids, groups := serviceTree(procs, 0, 50); len(pids) != 3 || !slices.Equal(groups, []int{300}) {
		t.Errorf("no root: pids=%v groups=%v", pids, groups)
	}
	if pids, _ := serviceTree(procs, 50, 50); slices.Contains(pids, 50) {
		t.Errorf("serviceTree returned self: %v", pids)
	}
}

// uniqueWorktree keeps a test's KIT_SERVICE tag from matching real services.
func uniqueWorktree(t *testing.T) string {
	return fmt.Sprintf("kit-test-%d-%d", os.Getpid(), time.Now().UnixNano())
}

func shortStopTimeouts(t *testing.T) {
	t.Helper()
	g, k := stopGrace, killWait
	stopGrace, killWait = 500*time.Millisecond, 2*time.Second
	t.Cleanup(func() { stopGrace, killWait = g, k })
}

// startTree launches script under bash as its own group leader, like
// StartService, with extra env. It returns the leader pid.
func startTree(t *testing.T, script string, env ...string) int {
	t.Helper()
	cmd := exec.Command("bash", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = append(os.Environ(), env...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	go func() { _ = cmd.Wait() }()
	t.Cleanup(func() { _ = syscall.Kill(-pid, syscall.SIGKILL) })
	return pid
}

// TestHelperProcess is not a test: startHelper re-runs the test binary into it
// to get a long-lived child whose environment ps -E can read. macOS hides the
// environment of platform binaries such as /bin/sleep and /bin/bash.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("KIT_TEST_HELPER") != "1" {
		t.Skip("helper process only")
	}
	time.Sleep(30 * time.Second)
	os.Exit(0)
}

// startHelper launches the helper as its own group leader with extra env.
func startHelper(t *testing.T, env ...string) int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = append(append(os.Environ(), "KIT_TEST_HELPER=1"), env...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	go func() { _ = cmd.Wait() }()
	t.Cleanup(func() { _ = syscall.Kill(-pid, syscall.SIGKILL) })
	time.Sleep(200 * time.Millisecond)
	return pid
}

// waitForFilePID waits for the script to write a pid to path.
func waitForFilePID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && pid > 0 {
				t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
				return pid
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no pid written to %s", path)
	return 0
}

// A descendant that moved to its own process group survives a group kill of
// the leader. StopService must still stop it.
func TestStopService_KillsDescendantOutsideGroup(t *testing.T) {
	setRunDir(t)
	shortStopTimeouts(t)
	wt := uniqueWorktree(t)
	pidPath := filepath.Join(t.TempDir(), "child.pid")
	// set -m gives the background job its own process group.
	leader := startTree(t, fmt.Sprintf("set -m; sleep 30 & echo $! > %q; wait", pidPath))
	child := waitForFilePID(t, pidPath)
	if pg, _ := syscall.Getpgid(child); pg == leader {
		t.Fatalf("test setup: child %d should not share the leader's group", child)
	}
	if err := WritePID(wt, string(SvcCelery), leader); err != nil {
		t.Fatal(err)
	}
	writeCmdStarted(t, wt, string(SvcCelery), time.Now())

	if err := StopService(wt, SvcCelery); err != nil {
		t.Fatalf("StopService = %v", err)
	}
	if live := livePIDs([]int{leader, child}); len(live) > 0 {
		t.Errorf("still alive after StopService: %v", live)
	}
	if ReadPID(wt, string(SvcCelery)) != 0 {
		t.Errorf("pidfile should be removed")
	}
}

// A worker kit launched but no longer tracks in its pidfile must still be
// stopped, or the restart runs a second worker next to it.
func TestStopService_KillsTaggedUntrackedProcess(t *testing.T) {
	setRunDir(t)
	shortStopTimeouts(t)
	wt := uniqueWorktree(t)
	pid := startHelper(t, serviceTag(wt, SvcCelery))

	if err := StopService(wt, SvcCelery); err != nil {
		t.Fatalf("StopService = %v", err)
	}
	if len(livePIDs([]int{pid})) > 0 {
		t.Errorf("tagged process %d survived", pid)
	}
}

func TestStopService_EscalatesToSIGKILL(t *testing.T) {
	setRunDir(t)
	shortStopTimeouts(t)
	wt := uniqueWorktree(t)
	pid := startTree(t, `trap "" TERM; while :; do sleep 0.05; done`, serviceTag(wt, SvcCelery))
	time.Sleep(100 * time.Millisecond)
	if err := WritePID(wt, string(SvcCelery), pid); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if err := StopService(wt, SvcCelery); err != nil {
		t.Fatalf("StopService = %v", err)
	}
	if len(livePIDs([]int{pid})) > 0 {
		t.Errorf("TERM-ignoring process %d survived", pid)
	}
	if time.Since(start) < stopGrace {
		t.Errorf("returned before the grace period; SIGTERM should have been ignored")
	}
}

// A tag must not reach other worktrees' or services' processes.
func TestStopService_LeavesOtherServicesAlone(t *testing.T) {
	setRunDir(t)
	shortStopTimeouts(t)
	wt := uniqueWorktree(t)
	beat := startHelper(t, serviceTag(wt, SvcBeat))
	other := startHelper(t, serviceTag(wt+"-other", SvcCelery))
	target := startHelper(t, serviceTag(wt, SvcCelery))

	if err := StopService(wt, SvcCelery); err != nil {
		t.Fatalf("StopService = %v", err)
	}
	if live := livePIDs([]int{beat, other}); len(live) != 2 {
		t.Errorf("unrelated processes killed; alive = %v of [%d %d]", live, beat, other)
	}
	if len(livePIDs([]int{target})) > 0 {
		t.Errorf("tagged target %d survived", target)
	}
}
