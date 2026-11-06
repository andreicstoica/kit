package liftoff

import (
	"testing"
	"time"
)

func TestIdleRequiresContinuousObservationAndResetsOnActivity(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	tracker := IdleTracker{}
	observation := IdleObservation{Name: "feature", Fingerprint: "revision-and-services", Activity: now.Add(-24 * time.Hour)}
	if tracker.Observe(observation, now, 2*time.Hour) {
		t.Fatal("old last_used must not pause on the first observation")
	}
	for i := 1; i < 24; i++ {
		if tracker.Observe(observation, now.Add(time.Duration(i)*5*time.Minute), 2*time.Hour) {
			t.Fatal("paused before two hours")
		}
	}
	if !tracker.Observe(observation, now.Add(2*time.Hour), 2*time.Hour) {
		t.Fatal("did not become eligible after two observed hours")
	}
	observation.Fingerprint = "edited-file"
	if tracker.Observe(observation, now.Add(125*time.Minute), 2*time.Hour) {
		t.Fatal("file change must restart the idle window")
	}
	observation.Blocked = "working agent"
	if tracker.Observe(observation, now.Add(130*time.Minute), 2*time.Hour) {
		t.Fatal("working agent must block pause")
	}
	observation.Blocked = ""
	if tracker.Observe(observation, now.Add(135*time.Minute), 2*time.Hour) {
		t.Fatal("idle period must restart after a blocker")
	}
	if tracker.Observe(observation, now.Add(4*time.Hour), 2*time.Hour) {
		t.Fatal("sleep or a missing observation must reset the window")
	}
}

func TestIdleUnknownAndRecentUseNeverQualify(t *testing.T) {
	now := time.Now()
	for _, blocked := range []string{"unknown terminal activity", "open client connection", "running job", "shared worker"} {
		tracker := IdleTracker{}
		o := IdleObservation{Name: "feature", Fingerprint: "same", Blocked: blocked}
		for i := 0; i <= 30; i++ {
			if tracker.Observe(o, now.Add(time.Duration(i)*5*time.Minute), 2*time.Hour) {
				t.Fatalf("paused despite %s", blocked)
			}
		}
	}
	tracker := IdleTracker{}
	o := IdleObservation{Name: "feature", Fingerprint: "same"}
	for i := 0; i < 24; i++ {
		tracker.Observe(o, now.Add(time.Duration(i)*5*time.Minute), 2*time.Hour)
	}
	o.Activity = now.Add(119 * time.Minute)
	if tracker.Observe(o, now.Add(120*time.Minute), 2*time.Hour) {
		t.Fatal("recent kit use must prevent pause")
	}
}

func TestIdleTerminalStatusRequiresMatchingOwner(t *testing.T) {
	now := time.Now()
	for _, test := range []struct {
		name, state string
		owner       int
		wantBlocked bool
	}{
		{"working", "working", 7, true}, {"unknown", "unexpected", 7, true}, {"old owner", "idle", 8, true}, {"idle", "idle", 7, false}, {"done", "done", 7, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := RexState{Sessions: []RexSession{{SessionID: "session", Windows: []RexWindow{{Blocks: []RexBlock{{Creator: "com.superlogical.terminal", Process: RexTerminalProcess{Foreground: &RexProcess{PID: 7, Name: "agent", Cwd: "/repo"}}, ProgramStatus: RexProgramStatus{Records: []RexProgramRecord{{State: test.state, Owner: &RexProcess{PID: test.owner, Name: "agent"}, UpdatedAt: now}}}}}}}}}}
			activity := time.Time{}
			allowed := map[int]bool{}
			reason := idleTerminalActivity(state, "session", "/repo", allowed, &activity)
			if (reason != "") != test.wantBlocked {
				t.Fatalf("block reason=%q", reason)
			}
			if !test.wantBlocked && (!allowed[7] || !activity.Equal(now)) {
				t.Fatal("verified idle owner and timestamp were not retained")
			}
		})
	}
}

func TestIdleAllowsOnlyAncestorsBetweenVerifiedTerminalProcesses(t *testing.T) {
	procs := map[int]procInfo{5: {PID: 5, PPID: 1}, 6: {PID: 6, PPID: 5}, 7: {PID: 7, PPID: 6}, 8: {PID: 8, PPID: 6}}
	allowed := map[int]bool{5: true, 7: true}
	allowIdleAncestors(procs, allowed)
	if !allowed[6] {
		t.Fatal("login shell between terminal root and idle agent was blocked")
	}
	if allowed[8] || allowed[1] {
		t.Fatal("background job or unrelated ancestor was admitted")
	}
}
