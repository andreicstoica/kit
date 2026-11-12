package liftoff

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mockPRLookup(t *testing.T, branch, oid, entries string) {
	t.Helper()
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "gh"), fmt.Sprintf(`#!/bin/sh
if [ "$1" = repo ]; then
  printf '{"nameWithOwner":"test/repo"}'
elif [ "$1" = api ]; then
  case "$*" in
    *'head=test:%s'*) printf '%%s' '%s' ;;
    *) echo 'missing branch head filter' >&2; exit 1 ;;
  esac
else
  printf '[{"headRefName":"%s","state":"OPEN","headRefOid":"%s"},{"headRefName":"%s","state":"CLOSED","headRefOid":"%s"}]'
fi
`, branch, entries, branch, oid, branch, oid))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestScanMergedWorktrees_PreservesBranchWithOpenPR(t *testing.T) {
	l := newMasterRepo(t)
	path := addWorktree(t, l, "reuse")
	commitAndPush(t, path, "reuse", "work.txt")
	runGit(t, l.Master, "merge", "--no-ff", "reuse", "--no-edit")
	head := runGit(t, path, "rev-parse", "HEAD")
	mockPRLookup(t, "reuse", head, fmt.Sprintf(`[[{"number":2,"state":"open","head":{"ref":"reuse","sha":"%s","repo":{"full_name":"test/repo"}}},{"number":1,"state":"closed","head":{"ref":"reuse","sha":"%s","repo":{"full_name":"test/repo"}}}]]`, head, head))
	scan, err := l.ScanMergedWorktrees()
	if err != nil || len(scan.Candidates) != 0 {
		t.Fatalf("cleanup candidates = %+v, %v; open PR must protect branch", scan.Candidates, err)
	}
}

func TestScanMergedWorktrees_ReportsPRLookupFailureAsWarning(t *testing.T) {
	l := newMasterRepo(t)
	landed := addWorktree(t, l, "landed")
	commitAndPush(t, landed, "landed", "l.txt")
	runGit(t, l.Master, "merge", "--no-ff", "landed", "--no-edit")
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "gh"), "#!/bin/sh\necho 'authentication failed' >&2\nexit 1\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	scan, err := l.ScanMergedWorktrees()
	if err != nil {
		t.Fatalf("PR lookup failure aborted the scan: %v", err)
	}
	if len(scan.Warnings) != 1 || !strings.Contains(scan.Warnings[0], "GitHub PR lookups failed") {
		t.Fatalf("warnings = %q, want a PR lookup warning", scan.Warnings)
	}
	if len(scan.Candidates) != 1 || scan.Candidates[0].Name != "landed" {
		t.Fatalf("locally merged worktree was dropped: %+v", scan.Candidates)
	}
}

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

func TestMergedBranchSet(t *testing.T) {
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

	m := mergedBranchSet(l.Master, "master", trunkRefs(l.Master, "master"))
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

// TestScanMergedWorktrees_SkipsUnpushed is the regression test for the wash
// bug: a freshly created worktree with no commits and no upstream must NOT be
// reported as merged, even though `git branch --merged` lists it (its tip is an
// ancestor of master). A branch that was actually merged AND pushed still is.
func TestScanMergedWorktrees_SkipsUnpushed(t *testing.T) {
	l := newMasterRepo(t)
	mockNoPRs(t)

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

	got, err := l.ScanMergedWorktrees()
	if err != nil {
		t.Fatal(err)
	}
	candidates := got.Candidates

	flagged := map[string]string{}
	for _, c := range candidates {
		flagged[c.Name] = c.Reason
	}

	if _, ok := flagged["fresh"]; ok {
		t.Error("ScanMergedWorktrees flagged 'fresh' — an unpushed, never-diverged worktree must not be washed")
	}
	if _, ok := flagged["wip"]; ok {
		t.Error("ScanMergedWorktrees flagged 'wip' — unmerged work must not be washed")
	}
	if _, ok := flagged["pushedEmpty"]; ok {
		t.Error("ScanMergedWorktrees flagged 'pushedEmpty' — a pushed branch sitting at the master tip never merged work and must not be washed")
	}
	if _, ok := flagged["master"]; ok {
		t.Error("ScanMergedWorktrees flagged the master worktree")
	}
	if _, ok := flagged["leftover"]; ok {
		t.Error("ScanMergedWorktrees flagged 'leftover' — a borrowed-upstream branch with only uncommitted work must not be washed")
	}
	if r := flagged["landed"]; r != "merged to master" {
		t.Errorf("ScanMergedWorktrees 'landed' reason = %q, want %q", r, "merged to master")
	}
}

// TestScanMergedWorktrees_MarksDirty: a genuinely merged worktree that still
// has uncommitted changes is a valid candidate but must carry Dirty=true so
// the picker can warn and default-deselect it.
func TestScanMergedWorktrees_MarksDirty(t *testing.T) {
	l := newMasterRepo(t)
	mockNoPRs(t)

	landed := addWorktree(t, l, "landed")
	commitAndPush(t, landed, "landed", "l.txt")
	runGit(t, l.Master, "merge", "--no-ff", "landed", "--no-edit")
	writeFile(t, landed, "leftover.txt", "uncommitted")

	clean := addWorktree(t, l, "clean")
	commitAndPush(t, clean, "clean", "c.txt")
	runGit(t, l.Master, "merge", "--no-ff", "clean", "--no-edit")

	scan, err := l.ScanMergedWorktrees()
	if err != nil {
		t.Fatal(err)
	}
	dirtyByName := map[string]bool{}
	for _, c := range scan.Candidates {
		dirtyByName[c.Name] = c.Dirty
		if c.Dirty && c.Safe() {
			t.Errorf("%s is dirty but reported safe to wash", c.Name)
		}
	}
	if d, ok := dirtyByName["landed"]; !ok || !d {
		t.Errorf("ScanMergedWorktrees 'landed' Dirty = %v (found=%v), want true", d, ok)
	}
	if d, ok := dirtyByName["clean"]; !ok || d {
		t.Errorf("ScanMergedWorktrees 'clean' Dirty = %v (found=%v), want false", d, ok)
	}
}

func TestMergedCandidateBlockersOwnSafetyRule(t *testing.T) {
	if c := (MergedCandidate{Kind: KindMergedIntoTrunk}); !c.Safe() || len(c.Blockers()) != 0 {
		t.Fatalf("merged candidate = %+v, blockers=%q; want safe", c, c.Blockers())
	}
	for _, tc := range []struct {
		name string
		c    MergedCandidate
	}{
		{"closed PR", MergedCandidate{Kind: KindClosedPR}},
		{"unknown kind", MergedCandidate{}},
		{"dirty", MergedCandidate{Kind: KindMergedPR, Dirty: true}},
		{"local work", MergedCandidate{Kind: KindMergedPR, LocalWork: true}},
		{"unverified", MergedCandidate{Kind: KindMergedPR, Unverified: true}},
	} {
		if tc.c.Safe() || len(tc.c.Blockers()) == 0 {
			t.Errorf("%s candidate = %+v; want unsafe with a blocker", tc.name, tc.c)
		}
	}
	all := MergedCandidate{Kind: KindMergedPR, Dirty: true, LocalWork: true, Unverified: true}
	if got := len(all.Blockers()); got != 3 {
		t.Fatalf("blockers = %q, want all three", all.Blockers())
	}
}

func TestTrunkAheadOfUnreadableTrunkIsFalse(t *testing.T) {
	l := newMasterRepo(t)
	if trunkAheadOf(l.Master, trunkRefs(l.Master, "missing-branch"), l.MainBranch) {
		t.Fatal("an unreadable trunk must never mark a branch for deletion")
	}
}

// TestScanMergedWorktrees_UsesFetchedOriginTrunk covers a merge that landed
// upstream while the local trunk still points at the pre-merge commit.
func TestScanMergedWorktrees_UsesFetchedOriginTrunk(t *testing.T) {
	l := newMasterRepo(t)
	mockNoPRs(t)

	landed := addWorktree(t, l, "landed")
	commitAndPush(t, landed, "landed", "l.txt")
	runGit(t, l.Master, "merge", "--no-ff", "landed", "--no-edit")
	runGit(t, l.Master, "push", "origin", "master")
	runGit(t, l.Master, "reset", "--hard", "HEAD~1")

	scan, err := l.ScanMergedWorktrees()
	if err != nil {
		t.Fatal(err)
	}
	if len(scan.Candidates) != 1 || scan.Candidates[0].Name != "landed" {
		t.Fatalf("origin trunk merge not detected: %+v", scan.Candidates)
	}
}
