package liftoff

import "testing"

func TestWorktreeName(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/Users/acs/liftoff/voice-agent", "voice-agent"},
		{"/Users/acs/liftoff/liftoff-voice-agent", "voice-agent"}, // legacy prefix stripped
		{"/Users/acs/liftoff/liftoff-app-master", "app-master"},
		{"/tmp/feat/", "feat"},
	}
	for _, tc := range cases {
		if got := (Worktree{Path: tc.path}).Name(); got != tc.want {
			t.Errorf("Worktree{%q}.Name() = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestIsMaster(t *testing.T) {
	l := Layout{Master: "/Users/acs/liftoff/liftoff-app-master"}
	cases := []struct {
		path string
		want bool
	}{
		{"/Users/acs/liftoff/liftoff-app-master", true},
		{"/Users/acs/liftoff/liftoff-app-master/", true}, // trailing slash tolerated
		{"/Users/acs/liftoff/voice-agent", false},
	}
	for _, tc := range cases {
		if got := (Worktree{Path: tc.path}).IsMaster(l); got != tc.want {
			t.Errorf("Worktree{%q}.IsMaster() = %v, want %v", tc.path, got, tc.want)
		}
	}
}

// TestListWorktrees verifies the porcelain parser against a real repo: it finds
// the master worktree plus an added one, with branches resolved.
func TestListWorktrees(t *testing.T) {
	l := newMasterRepo(t)
	addWorktree(t, l, "feature-x")

	wts, err := l.ListWorktrees()
	if err != nil {
		t.Fatal(err)
	}

	byBranch := map[string]Worktree{}
	for _, w := range wts {
		byBranch[w.Branch] = w
	}

	master, ok := byBranch["master"]
	if !ok {
		t.Fatalf("master worktree not listed; got %+v", wts)
	}
	if !master.IsMaster(l) {
		t.Errorf("master worktree IsMaster() = false for path %q", master.Path)
	}

	feat, ok := byBranch["feature-x"]
	if !ok {
		t.Fatalf("added worktree 'feature-x' not listed; got %+v", wts)
	}
	if feat.Name() != "feature-x" {
		t.Errorf("feature worktree Name() = %q, want %q", feat.Name(), "feature-x")
	}
	if feat.Head == "" {
		t.Error("feature worktree HEAD sha not parsed")
	}
}
