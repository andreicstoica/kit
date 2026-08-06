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

func TestBranchHasOwnUpstream(t *testing.T) {
	l := newMasterRepo(t)

	// pushed branch: tracks origin/<its own name>.
	pushed := addWorktree(t, l, "pushed")
	commitAndPush(t, pushed, "pushed", "p.txt")
	if !branchHasOwnUpstream(l.Master, "pushed") {
		t.Error("branchHasOwnUpstream(pushed) = false, want true (tracks origin/pushed)")
	}

	// local-only branch: never pushed, no upstream.
	addWorktree(t, l, "local")
	if branchHasOwnUpstream(l.Master, "local") {
		t.Error("branchHasOwnUpstream(local) = true, want false (no upstream)")
	}

	// borrowed upstream: tracks origin/master, never pushed itself. This is
	// how a worktree branch looks before its first push — it must not count
	// as pushed.
	addWorktree(t, l, "borrowed")
	runGit(t, l.Master, "branch", "--set-upstream-to=origin/master", "borrowed")
	if branchHasOwnUpstream(l.Master, "borrowed") {
		t.Error("branchHasOwnUpstream(borrowed) = true, want false (upstream is origin/master, not its own)")
	}

	// nonexistent branch.
	if branchHasOwnUpstream(l.Master, "ghost") {
		t.Error("branchHasOwnUpstream(ghost) = true, want false")
	}
}

func TestMergedBranches(t *testing.T) {
	l := newMasterRepo(t)

	// A branch whose commits are merged into master.
	landed := addWorktree(t, l, "landed")
	commitAndPush(t, landed, "landed", "l.txt")
	runGit(t, l.Master, "merge", "--no-ff", "landed", "--no-edit")

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
	runGit(t, l.Master, "merge", "--no-ff", "landed", "--no-edit")

	// fresh: just created off master, no commits, never pushed → must be
	// skipped despite `git branch --merged` listing it.
	addWorktree(t, l, "fresh")

	// wip: unmerged local work → not merged, must be skipped.
	wip := addWorktree(t, l, "wip")
	writeFile(t, wip, "w.txt", "wip")
	runGit(t, wip, "add", ".")
	runGit(t, wip, "commit", "-m", "wip")

	// pushedEmpty: a worktree pushed before any commit, so its tip sits exactly
	// at master. `git branch --merged` lists it and it has an upstream, but no
	// work ever landed → must be skipped (regression: work uncommitted in tree).
	addWorktree(t, l, "pushedEmpty")
	runGit(t, l.Master, "push", "-u", "origin", "pushedEmpty")

	// leftover: parked at an old master tip with a BORROWED upstream
	// (origin/master) and only uncommitted work in the tree. master is
	// strictly ahead (the landed merge above), `git branch --merged` lists
	// it, and it "has an upstream" — the exact signal-docs-dedupe shape.
	// Must be skipped.
	leftover := addWorktree(t, l, "leftover")
	runGit(t, l.Master, "branch", "--set-upstream-to=origin/master", "leftover")
	writeFile(t, leftover, "wip.txt", "staged but never committed")
	runGit(t, leftover, "add", ".")

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
	if _, ok := flagged["pushedEmpty"]; ok {
		t.Error("FindMergedWorktrees flagged 'pushedEmpty' — a pushed branch sitting at the master tip never merged work and must not be washed")
	}
	if _, ok := flagged["master"]; ok {
		t.Error("FindMergedWorktrees flagged the master worktree")
	}
	if _, ok := flagged["leftover"]; ok {
		t.Error("FindMergedWorktrees flagged 'leftover' — a borrowed-upstream branch with only uncommitted work must not be washed")
	}
	if r := flagged["landed"]; r != "merged to master" {
		t.Errorf("FindMergedWorktrees 'landed' reason = %q, want %q", r, "merged to master")
	}
}

// TestFindMergedWorktrees_MarksDirty: a genuinely merged worktree that still
// has uncommitted changes is a valid candidate but must carry Dirty=true so
// the picker can warn and default-deselect it.
func TestFindMergedWorktrees_MarksDirty(t *testing.T) {
	l := newMasterRepo(t)

	landed := addWorktree(t, l, "landed")
	commitAndPush(t, landed, "landed", "l.txt")
	runGit(t, l.Master, "merge", "--no-ff", "landed", "--no-edit")
	writeFile(t, landed, "leftover.txt", "uncommitted")

	clean := addWorktree(t, l, "clean")
	commitAndPush(t, clean, "clean", "c.txt")
	runGit(t, l.Master, "merge", "--no-ff", "clean", "--no-edit")

	got, err := l.FindMergedWorktrees()
	if err != nil {
		t.Fatal(err)
	}
	dirtyByName := map[string]bool{}
	for _, c := range got {
		dirtyByName[c.Name] = c.Dirty
	}
	if d, ok := dirtyByName["landed"]; !ok || !d {
		t.Errorf("FindMergedWorktrees 'landed' Dirty = %v (found=%v), want true", d, ok)
	}
	if d, ok := dirtyByName["clean"]; !ok || d {
		t.Errorf("FindMergedWorktrees 'clean' Dirty = %v (found=%v), want false", d, ok)
	}
}
