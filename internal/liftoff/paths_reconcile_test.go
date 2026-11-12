package liftoff

import "testing"

func TestParseWorktrees(t *testing.T) {
	out := "worktree /Users/acs/liftoff/liftoff-app-master\n" +
		"HEAD abc123\n" +
		"branch refs/heads/master\n" +
		"\n" +
		"worktree /Users/acs/liftoff/new-signals\n" +
		"HEAD def456\n" +
		"branch refs/heads/feat/new-signals\n" +
		"\n" +
		"worktree /Users/acs/liftoff/detached-one\n" +
		"HEAD 789abc\n" +
		"detached\n" +
		"\n" +
		"worktree /Users/acs/liftoff/locked-one\n" +
		"HEAD fedcba\n" +
		"branch refs/heads/locked\n" +
		"locked external disk\n"

	got := parseWorktrees(out)
	if len(got) != 4 {
		t.Fatalf("want 4 entries, got %d: %+v", len(got), got)
	}
	if got[0].Path != "/Users/acs/liftoff/liftoff-app-master" || got[0].Branch != "master" || got[0].Head != "abc123" {
		t.Errorf("entry 0 = %+v", got[0])
	}
	if got[1].Path != "/Users/acs/liftoff/new-signals" || got[1].Branch != "feat/new-signals" {
		t.Errorf("entry 1 = %+v", got[1])
	}
	// Detached HEAD: path and commit present, branch empty.
	if got[2].Path != "/Users/acs/liftoff/detached-one" || got[2].Branch != "" || got[2].Head != "789abc" || !got[2].Detached {
		t.Errorf("entry 2 = %+v", got[2])
	}
	if !got[3].Locked {
		t.Errorf("entry 3 = %+v", got[3])
	}
}

func TestParseWorktreesEmpty(t *testing.T) {
	if got := parseWorktrees(""); len(got) != 0 {
		t.Errorf("empty input should yield no entries, got %+v", got)
	}
}

func TestParseWorktreesToleratesMissingSeparator(t *testing.T) {
	out := "worktree /a\nHEAD 1\nbranch refs/heads/a\n" +
		"worktree /b\nHEAD 2\nbranch refs/heads/b\n"
	got := parseWorktrees(out)
	if len(got) != 2 || got[0].Path != "/a" || got[1].Path != "/b" {
		t.Fatalf("missing blank-line separator merged records: %+v", got)
	}
}
