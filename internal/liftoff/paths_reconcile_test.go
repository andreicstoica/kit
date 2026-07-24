package liftoff

import "testing"

func TestParseWorktreePorcelain(t *testing.T) {
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
		"detached\n"

	got := parseWorktreePorcelain(out)
	if len(got) != 3 {
		t.Fatalf("want 3 entries, got %d: %+v", len(got), got)
	}
	if got[0].Path != "/Users/acs/liftoff/liftoff-app-master" || got[0].Branch != "master" {
		t.Errorf("entry 0 = %+v", got[0])
	}
	if got[1].Path != "/Users/acs/liftoff/new-signals" || got[1].Branch != "feat/new-signals" {
		t.Errorf("entry 1 = %+v", got[1])
	}
	// Detached HEAD: path present, branch empty.
	if got[2].Path != "/Users/acs/liftoff/detached-one" || got[2].Branch != "" {
		t.Errorf("entry 2 = %+v", got[2])
	}
}

func TestParseWorktreePorcelainEmpty(t *testing.T) {
	if got := parseWorktreePorcelain(""); len(got) != 0 {
		t.Errorf("empty input should yield no entries, got %+v", got)
	}
}
