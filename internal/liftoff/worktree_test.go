package liftoff

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestPathIdentityResolvesSymlinks guards the master checkout protection when
// the layout and git spell the same directory differently.
func TestPathIdentityResolvesSymlinks(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	l := Layout{Master: real}
	if !(Worktree{Path: alias}).IsMaster(l) {
		t.Fatal("symlinked master checkout was not recognized as master")
	}
	set := NewWorktreeSet([]Worktree{{Path: real}})
	if _, ok := set.At(alias); !ok {
		t.Fatal("a symlinked checkout path did not resolve to the worktree")
	}
}

func TestFastForwardTrunkReportsOnlyRealMovement(t *testing.T) {
	l := newMasterRepo(t)
	ran, err := l.FastForwardTrunk(nil)
	if err != nil || ran {
		t.Fatalf("already-current trunk: ran=%v err=%v, want no movement", ran, err)
	}

	remote := runGit(t, l.Master, "remote", "get-url", "origin")
	other := filepath.Join(t.TempDir(), "other")
	if out, err := exec.Command("git", "clone", remote, other).CombinedOutput(); err != nil {
		t.Fatalf("clone fixture: %v\n%s", err, out)
	}
	runGit(t, other, "config", "user.email", "test@example.com")
	runGit(t, other, "config", "user.name", "Test")
	writeFile(t, other, "upstream.txt", "upstream")
	runGit(t, other, "add", ".")
	runGit(t, other, "commit", "-m", "upstream")
	runGit(t, other, "push", "origin", "master")
	if err := l.FetchMain(nil); err != nil {
		t.Fatal(err)
	}
	ran, err = l.FastForwardTrunk(nil)
	if err != nil || !ran {
		t.Fatalf("stale trunk: ran=%v err=%v, want a fast-forward", ran, err)
	}
}

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

func TestIsDirty_UnknownStatusIsNotClean(t *testing.T) {
	if !IsDirty(t.TempDir()) {
		t.Fatal("Git status failure must not mark a checkout as clean")
	}
}

// TestWorktreeSetMatchesByPathKeepsMovedCheckout guards the path arm of the
// config-record resolver: a checkout renamed on disk and on its branch must
// still match the recorded path.
func TestWorktreeSetMatchesByPathKeepsMovedCheckout(t *testing.T) {
	path := t.TempDir() + "/moved-checkout"
	set := NewWorktreeSet([]Worktree{{Path: path, Branch: "renamed"}})
	if _, ok := set.Find("original", "original", path); !ok {
		t.Fatal("WorktreeSet.Find did not match the recorded checkout path")
	}
	if _, ok := set.At(path); !ok {
		t.Fatal("WorktreeSet.At did not match the recorded checkout path")
	}
}
