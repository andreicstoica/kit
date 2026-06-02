package liftoff

import "testing"

// commitAndPush commits in the worktree and pushes with -u so the branch gets
// an upstream.
func commitAndPush(t *testing.T, worktree, branch, file string) {
	t.Helper()
	writeFile(t, worktree, file, "change")
	runGit(t, worktree, "add", ".")
	runGit(t, worktree, "commit", "-m", "work on "+branch)
	runGit(t, worktree, "push", "-u", "origin", branch)
}

func TestBranchPushed(t *testing.T) {
	l := newMasterRepo(t)

	// pushed branch: has an upstream.
	pushed := addWorktree(t, l, "pushed")
	commitAndPush(t, pushed, "pushed", "p.txt")
	if !branchPushed(l.Master, "pushed") {
		t.Error("branchPushed(pushed) = false, want true (has upstream)")
	}

	// local-only branch: never pushed, no upstream.
	addWorktree(t, l, "local")
	if branchPushed(l.Master, "local") {
		t.Error("branchPushed(local) = true, want false (no upstream)")
	}

	// nonexistent branch.
	if branchPushed(l.Master, "ghost") {
		t.Error("branchPushed(ghost) = true, want false")
	}
}

func TestMergedBranches(t *testing.T) {
	l := newMasterRepo(t)

	// A branch whose commits are merged into master.
	landed := addWorktree(t, l, "landed")
	commitAndPush(t, landed, "landed", "l.txt")
	runGit(t, l.Master, "merge", "landed", "--no-edit")

	// A branch with unmerged work.
	wip := addWorktree(t, l, "wip")
	writeFile(t, wip, "w.txt", "wip")
	runGit(t, wip, "add", ".")
	runGit(t, wip, "commit", "-m", "wip")

	m := mergedBranches(l.Master, "master")
	if !m["landed"] {
		t.Error("mergedBranches missing 'landed' (it was merged into master)")
	}
	if m["wip"] {
		t.Error("mergedBranches included 'wip' (it has unmerged commits)")
	}
	if m["master"] {
		t.Error("mergedBranches must exclude the main branch itself")
	}
}

// TestFindMergedWorktrees_SkipsUnpushed is the regression test for the wash
// bug: a freshly created worktree with no commits and no upstream must NOT be
// reported as merged, even though `git branch --merged` lists it (its tip is an
// ancestor of master). A branch that was actually merged AND pushed still is.
func TestFindMergedWorktrees_SkipsUnpushed(t *testing.T) {
	l := newMasterRepo(t)

	// landed: merged into master and pushed → should be flagged.
	landed := addWorktree(t, l, "landed")
	commitAndPush(t, landed, "landed", "l.txt")
	runGit(t, l.Master, "merge", "landed", "--no-edit")

	// fresh: just created off master, no commits, never pushed → must be
	// skipped despite `git branch --merged` listing it.
	addWorktree(t, l, "fresh")

	// wip: unmerged local work → not merged, must be skipped.
	wip := addWorktree(t, l, "wip")
	writeFile(t, wip, "w.txt", "wip")
	runGit(t, wip, "add", ".")
	runGit(t, wip, "commit", "-m", "wip")

	got, err := l.FindMergedWorktrees()
	if err != nil {
		t.Fatal(err)
	}

	flagged := map[string]string{}
	for _, c := range got {
		flagged[c.Name] = c.Reason
	}

	if _, ok := flagged["fresh"]; ok {
		t.Error("FindMergedWorktrees flagged 'fresh' — an unpushed, never-diverged worktree must not be washed")
	}
	if _, ok := flagged["wip"]; ok {
		t.Error("FindMergedWorktrees flagged 'wip' — unmerged work must not be washed")
	}
	if _, ok := flagged["master"]; ok {
		t.Error("FindMergedWorktrees flagged the master worktree")
	}
	if r := flagged["landed"]; r != "merged to master" {
		t.Errorf("FindMergedWorktrees 'landed' reason = %q, want %q", r, "merged to master")
	}
}
