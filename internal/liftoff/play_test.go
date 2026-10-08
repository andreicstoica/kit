package liftoff

import (
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRunPlayFrontendsDoNotWaitForBroker(t *testing.T) {
	setRunDir(t)
	t.Setenv("CELERY_BROKER_URL", "")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	old := rabbitmqctl
	rabbitmqctl = func(args ...string) ([]byte, error) { <-release; return nil, nil }
	t.Cleanup(func() { rabbitmqctl = old })
	port := listener.Addr().(*net.TCPAddr).Port
	// The API must be down, or no backend needs the broker and kit skips it.
	idle, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	apiPort := idle.Addr().(*net.TCPAddr).Port
	idle.Close()
	updates := (Layout{}).RunPlay(PlayPlan{Worktree: "frontend-broker-test", WorktreePath: t.TempDir(), Ports: Ports{App: port, API: apiPort}, Services: []Service{SvcApp, SvcAPI}})
	defer func() {
		unblock()
		for range updates {
		}
	}()
	select {
	case u := <-updates:
		if u.Service != SvcApp || u.Status != StepDone {
			t.Fatalf("expected frontend before broker, got %+v", u)
		}
	case <-time.After(time.Second):
		unblock()
		for range updates {
		}
		t.Fatal("frontend startup waits for RabbitMQ even though it does not use it")
	}
	unblock()
	for range updates {
	}
}

// TestRunPlay_SkipsAlreadyListening guards `kit play` idempotency: a service
// whose port is already listening must be reported "already running" rather
// than restarted — otherwise play spawns a duplicate that can't bind the port
// and hangs (the IPv6/Vite regression).
func TestRunPlay_SkipsAlreadyListening(t *testing.T) {
	setRunDir(t)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	plan := PlayPlan{
		Worktree:     "fake",
		WorktreePath: "/nonexistent",
		Ports:        Ports{App: port},
		Services:     []Service{SvcApp},
	}

	var updates []PlayUpdate
	for u := range (Layout{}).RunPlay(plan) {
		updates = append(updates, u)
	}

	if len(updates) != 1 {
		t.Fatalf("expected 1 update (skip), got %d: %+v", len(updates), updates)
	}
	u := updates[0]
	if u.Status != StepDone {
		t.Errorf("status = %v, want StepDone", u.Status)
	}
	if !strings.Contains(u.Title, "already running") {
		t.Errorf("title = %q, want it to mention 'already running'", u.Title)
	}
}

// TestRunPause_SkipsWhenNoPIDAndNoListener: no recorded pid and nothing bound
// on the service's port → StepSkipped (nothing to stop).
func TestRunPause_SkipsWhenNoPIDAndNoListener(t *testing.T) {
	setRunDir(t)

	// Grab a free port, then close it so nothing is listening.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	plan := PausePlan{
		Worktree: "fake",
		Services: []Service{SvcApp},
		Ports:    Ports{App: port},
	}

	var updates []PlayUpdate
	for u := range (Layout{}).RunPause(plan) {
		updates = append(updates, u)
	}

	if len(updates) != 1 {
		t.Fatalf("expected 1 update (skip), got %d: %+v", len(updates), updates)
	}
	u := updates[0]
	if u.Status != StepSkipped {
		t.Errorf("status = %v, want StepSkipped", u.Status)
	}
	if !strings.Contains(u.Title, "not running") {
		t.Errorf("title = %q, want it to mention 'not running'", u.Title)
	}
}

// A worker on a shared broker must not start beside a live shared worker,
// whether celery or only beat is selected.
func TestRunPlaySkipsCeleryAndBeatOnSharedBrokerWithLiveOwner(t *testing.T) {
	for _, svcs := range [][]Service{{SvcCelery}, {SvcBeat}, {SvcCelery, SvcBeat}} {
		setRunDir(t)
		t.Setenv("CELERY_BROKER_URL", "pyamqp://u@remote.example/prod") // user-set: not isolated
		if err := WritePID("other", string(SvcCelery), os.Getpid()); err != nil {
			t.Fatal(err)
		}
		var updates []PlayUpdate
		for u := range (Layout{}).RunPlay(PlayPlan{Worktree: "mine", WorktreePath: t.TempDir(), Services: svcs}) {
			updates = append(updates, u)
		}
		skipped := 0
		for _, u := range updates {
			if u.Status == StepRunning || u.Status == StepDone && u.Service != "" {
				t.Fatalf("%v started despite live shared worker: %+v", svcs, u)
			}
			if u.Status == StepSkipped && u.Service != "" {
				skipped++
			}
		}
		if skipped != len(svcs) {
			t.Fatalf("%v: skipped %d services, want %d: %+v", svcs, skipped, len(svcs), updates)
		}
	}
}

func TestFindSharedCeleryOwnerIgnoresPrivateVhostWorkers(t *testing.T) {
	setRunDir(t)
	if err := WritePID("isolated", string(SvcCelery), os.Getpid()); err != nil {
		t.Fatal(err)
	}
	cmd, err := CmdFile("isolated", string(SvcCelery))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cmd, []byte("env: CELERY_BROKER_URL="+BrokerURL("kit-isolated")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if owner, _ := FindSharedCeleryOwner("mine"); owner != "" {
		t.Fatalf("worker on its own vhost reported as shared owner: %q", owner)
	}
	if err := os.WriteFile(cmd, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	if owner, _ := FindSharedCeleryOwner("mine"); owner != "isolated" {
		t.Fatalf("shared worker not found, got %q", owner)
	}
}
