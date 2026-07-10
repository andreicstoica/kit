package liftoff

import (
	"net"
	"strings"
	"testing"
)

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
