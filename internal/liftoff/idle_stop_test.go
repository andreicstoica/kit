package liftoff

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestIdleSignalHelper(t *testing.T) {
	ready := os.Getenv("KIT_IDLE_SIGNAL_HELPER")
	if ready == "" {
		t.Skip("helper process only")
	}
	received := make(chan os.Signal, 1)
	signal.Notify(received, syscall.SIGTERM)
	if err := os.WriteFile(ready, []byte("ready"), 0600); err != nil {
		os.Exit(2)
	}
	<-received
	if err := os.WriteFile(ready+".term", []byte("received"), 0600); err != nil {
		os.Exit(3)
	}
	if os.Getenv("KIT_IDLE_IGNORE_TERM") == "1" {
		time.Sleep(time.Minute)
	}
	os.Exit(0)
}

func idleSignalHelper(t *testing.T, name string, svc Service, ignore bool) (int, string) {
	t.Helper()
	ready := filepath.Join(t.TempDir(), "ready")
	cmd := exec.Command(os.Args[0], "-test.run=^TestIdleSignalHelper$")
	cmd.Env = append(os.Environ(), "KIT_IDLE_SIGNAL_HELPER="+ready, serviceTag(name, svc))
	if ignore {
		cmd.Env = append(cmd.Env, "KIT_IDLE_IGNORE_TERM=1")
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	go func() { _ = cmd.Wait() }()
	t.Cleanup(func() { _ = syscall.Kill(-pid, syscall.SIGKILL) })
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := WritePID(name, string(svc), pid); err != nil {
		t.Fatal(err)
	}
	return pid, ready
}

func TestIdleStopNeverEscalatesLongWork(t *testing.T) {
	setStateDir(t)
	setRunDir(t)
	shortStopTimeouts(t)
	name := uniqueWorktree(t)
	pid, ready := idleSignalHelper(t, name, SvcCelery, true)
	err := stopIdleService(name, SvcCelery, pid)
	if err == nil || !strings.Contains(err.Error(), "no SIGKILL") {
		t.Fatalf("expected draining result, got %v", err)
	}
	if _, err := os.Stat(ready + ".term"); err != nil {
		t.Fatal("worker did not receive graceful shutdown")
	}
	if !IsAlive(pid) || ReadPID(name, string(SvcCelery)) != pid {
		t.Fatal("long task or its ownership record was removed")
	}
}

func TestIdleStopRejectsChangedPIDAndPreservesOtherServices(t *testing.T) {
	setStateDir(t)
	setRunDir(t)
	name := uniqueWorktree(t)
	pid, _ := idleSignalHelper(t, name, SvcApp, false)
	other, _ := idleSignalHelper(t, name, SvcAdmin, false)
	if err := stopIdleService(name, SvcApp, pid+1); err == nil {
		t.Fatal("accepted stale observation")
	}
	if !IsAlive(pid) {
		t.Fatal("changed observation stopped service")
	}
	if err := stopIdleService(name, SvcApp, pid); err != nil {
		t.Fatal(err)
	}
	if IsAlive(pid) || ReadPID(name, string(SvcApp)) != 0 {
		t.Fatal("owned service did not stop")
	}
	if !IsAlive(other) {
		t.Fatal("unrelated service was stopped")
	}
}
