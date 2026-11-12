package liftoff

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mergedPR reports one merged PR for branch "work".
func mergedPR(sha string) string {
	return fmt.Sprintf(`[[{"number":3,"state":"closed","merged_at":"2026-01-01T00:00:00Z","head":{"ref":"work","sha":"%s","repo":{"full_name":"test/repo"}}}]]`, sha)
}

func mockNoPRs(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "gh"), `#!/bin/sh
if [ "$1" = repo ]; then
  printf '{"nameWithOwner":"test/repo"}'
else
  printf '[[]]'
fi
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestRemotePRStatuses_HistoryAndFailures(t *testing.T) {
	for _, tc := range []struct {
		name, entries, want string
		number              int
		wantErr             bool
	}{
		{"no PR", `[[]]`, "NONE", 0, false},
		{"latest closed", `[[{"number":2,"state":"closed","head":{"ref":"work","sha":"abc","repo":{"full_name":"test/repo"}}},{"number":1,"state":"closed","merged_at":"2026-01-01T00:00:00Z","head":{"ref":"work","repo":{"full_name":"test/repo"}}}]]`, "CLOSED", 2, false},
		{"open on later page", `[[{"number":2,"state":"closed","head":{"ref":"work","repo":{"full_name":"test/repo"}}}],[{"number":1,"state":"open","head":{"ref":"work","repo":{"full_name":"test/repo"}}}]]`, "OPEN", 1, false},
		{"fork ignored", `[[{"number":3,"state":"closed","head":{"ref":"work","repo":{"full_name":"other/repo"}}},{"number":2,"state":"open","head":{"ref":"work","repo":{"full_name":"test/repo"}}}]]`, "OPEN", 2, false},
		{"malformed", `invalid JSON`, "UNKNOWN", 0, true},
		{"invalid state", `[[{"number":3,"state":"unexpected","head":{"ref":"work","repo":{"full_name":"test/repo"}}}]]`, "UNKNOWN", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := Layout{Master: t.TempDir()}
			mockPRLookup(t, "work", "abc", tc.entries)
			got, err := l.RemotePRStatuses([]string{"work", "work", ""})
			if (err != nil) != tc.wantErr {
				t.Fatalf("lookup error = %v, want error=%v", err, tc.wantErr)
			}
			if len(got) != 1 || got["work"].State != tc.want || got["work"].Number != tc.number {
				t.Fatalf("statuses = %+v, want work #%d %s", got, tc.number, tc.want)
			}
		})
	}
}

func TestScanMergedWorktrees_KindClosedPRRequiresExactHead(t *testing.T) {
	l := newMasterRepo(t)
	path := addWorktree(t, l, "work")
	writeFile(t, path, "work.txt", "submitted")
	runGit(t, path, "add", ".")
	runGit(t, path, "commit", "-m", "submitted")
	head := runGit(t, path, "rev-parse", "HEAD")
	mockPRLookup(t, "work", head, fmt.Sprintf(`[[{"number":1,"state":"closed","head":{"ref":"work","sha":"%s","repo":{"full_name":"test/repo"}}}]]`, head))
	scan, err := l.ScanMergedWorktrees()
	if err != nil || len(scan.Candidates) != 1 || scan.Candidates[0].Kind != KindClosedPR {
		t.Fatalf("matching closed PR candidates = %+v, %v; want one", scan.Candidates, err)
	}
	if scan.Candidates[0].Safe() {
		t.Fatal("closed PR reported safe to wash automatically")
	}
	writeFile(t, path, "new.txt", "new local work")
	runGit(t, path, "add", ".")
	runGit(t, path, "commit", "-m", "new work after PR")
	scan, err = l.ScanMergedWorktrees()
	if err != nil || len(scan.Candidates) != 0 {
		t.Fatalf("new local commit candidates = %+v, %v; want none", scan.Candidates, err)
	}
}

// A squash merge leaves the branch commits out of trunk's ancestry. Comparing
// content instead of commits is what proves the work landed.
func TestScanMergedWorktrees_SquashMergedContentIsSafe(t *testing.T) {
	l := newMasterRepo(t)
	path := addWorktree(t, l, "work")
	writeFile(t, path, "work.txt", "submitted")
	runGit(t, path, "add", ".")
	runGit(t, path, "commit", "-m", "submitted")
	head := runGit(t, path, "rev-parse", "HEAD")
	runGit(t, l.Master, "merge", "--squash", "work")
	runGit(t, l.Master, "commit", "-m", "squash work")
	// Rewrite the local head so the PR head cannot be the proof.
	runGit(t, path, "commit", "--amend", "-m", "reworded after merge")

	mockPRLookup(t, "work", head, mergedPR(head))
	scan, err := l.ScanMergedWorktrees()
	if err != nil || len(scan.Candidates) != 1 {
		t.Fatalf("squash-merged candidates = %+v, %v; want one", scan.Candidates, err)
	}
	c := scan.Candidates[0]
	if c.Kind != KindMergedPR || c.Unverified || c.LocalWork || !c.Safe() {
		t.Fatalf("candidate = %+v; want a safe squash-merged candidate", c)
	}
}

// A merge queue rewrites the branch commits, so the PR head no longer equals
// the local SHA even though the same content landed.
func TestScanMergedWorktrees_RewrittenMergedHeadIsSafe(t *testing.T) {
	l := newMasterRepo(t)
	path := addWorktree(t, l, "work")
	writeFile(t, path, "a.txt", "a")
	runGit(t, path, "add", ".")
	runGit(t, path, "commit", "-m", "a")
	writeFile(t, path, "b.txt", "b")
	runGit(t, path, "add", ".")
	runGit(t, path, "commit", "-m", "b")
	branchHead := runGit(t, path, "rev-parse", "HEAD")

	writeFile(t, l.Master, "trunk.txt", "trunk")
	runGit(t, l.Master, "add", ".")
	runGit(t, l.Master, "commit", "-m", "trunk moved")
	runGit(t, l.Master, "checkout", "-b", "merged", "work")
	runGit(t, l.Master, "rebase", "master")
	runGit(t, l.Master, "checkout", "master")
	runGit(t, l.Master, "merge", "--ff-only", "merged")
	runGit(t, l.Master, "branch", "-D", "merged")
	if runGit(t, l.Master, "rev-parse", "master") == branchHead {
		t.Fatal("fixture did not rewrite the merged commits")
	}
	// Rewrite the local head too; only content comparison can prove the merge.
	runGit(t, path, "commit", "--amend", "-m", "reworded after merge")

	mockPRLookup(t, "work", branchHead, mergedPR(branchHead))
	scan, err := l.ScanMergedWorktrees()
	if err != nil || len(scan.Candidates) != 1 {
		t.Fatalf("rewritten-head candidates = %+v, %v; want one", scan.Candidates, err)
	}
	c := scan.Candidates[0]
	if c.Kind != KindMergedPR || c.Unverified || c.LocalWork || !c.Safe() {
		t.Fatalf("candidate = %+v; want a safe candidate despite the rewritten head", c)
	}
}

// A commit pushed after the merge is not in trunk; the worktree must stay
// visible and unsafe even though the PR head matches the checkout.
func TestScanMergedWorktrees_KeepsPostMergeCommitsVisible(t *testing.T) {
	l := newMasterRepo(t)
	path := addWorktree(t, l, "work")
	writeFile(t, path, "shipped.txt", "shipped")
	runGit(t, path, "add", ".")
	runGit(t, path, "commit", "-m", "shipped work")
	runGit(t, l.Master, "merge", "--squash", "work")
	runGit(t, l.Master, "commit", "-m", "squash work")

	writeFile(t, path, "after.txt", "after merge")
	runGit(t, path, "add", ".")
	runGit(t, path, "commit", "-m", "work after merge")
	head := runGit(t, path, "rev-parse", "HEAD")

	mockPRLookup(t, "work", head, mergedPR(head))
	scan, err := l.ScanMergedWorktrees()
	if err != nil || len(scan.Candidates) != 1 {
		t.Fatalf("post-merge commit candidates = %+v, %v; want the worktree kept visible", scan.Candidates, err)
	}
	c := scan.Candidates[0]
	if c.Kind != KindMergedPR || c.Safe() || !c.LocalWork {
		t.Fatalf("candidate = %+v; post-merge commits must be surfaced as unsafe", c)
	}
}

// A merge driver can make git's merge result equal trunk while discarding the
// checkout's version, so a configured driver must block the content proof.
func TestScanMergedWorktrees_CustomMergeDriverIsUnverified(t *testing.T) {
	l := newMasterRepo(t)
	writeFile(t, l.Master, ".gitattributes", "gen.lock merge=ours\n")
	runGit(t, l.Master, "add", ".")
	runGit(t, l.Master, "commit", "-m", "merge driver")
	runGit(t, l.Master, "config", "merge.ours.driver", "true")

	path := addWorktree(t, l, "work")
	writeFile(t, path, "gen.lock", "branch-version")
	runGit(t, path, "add", ".")
	runGit(t, path, "commit", "-m", "branch lock")
	head := runGit(t, path, "rev-parse", "HEAD")
	writeFile(t, l.Master, "gen.lock", "trunk-version")
	runGit(t, l.Master, "add", ".")
	runGit(t, l.Master, "commit", "-m", "trunk lock")

	mockPRLookup(t, "work", head, mergedPR(head))
	scan, err := l.ScanMergedWorktrees()
	if err != nil || len(scan.Candidates) != 1 {
		t.Fatalf("merge-driver candidates = %+v, %v; want one", scan.Candidates, err)
	}
	c := scan.Candidates[0]
	if !c.Unverified || c.Safe() {
		t.Fatalf("candidate = %+v; a merge driver must block the content proof", c)
	}
}

// When trunk cannot be read there is no safe proof, so the candidate is kept
// but never selected by default.
func TestScanMergedWorktrees_UnverifiedWhenTrunkIsUnreadable(t *testing.T) {
	l := newMasterRepo(t)
	l.MainBranch = "missing-branch"
	addWorktree(t, l, "work")
	mockPRLookup(t, "work", strings.Repeat("ab", 20), mergedPR(strings.Repeat("ab", 20)))
	scan, err := l.ScanMergedWorktrees()
	if err != nil || len(scan.Candidates) != 1 {
		t.Fatalf("unverifiable candidates = %+v, %v; want one", scan.Candidates, err)
	}
	c := scan.Candidates[0]
	if c.Kind != KindMergedPR || !c.Unverified || c.Safe() {
		t.Fatalf("candidate = %+v; want an unverified, unsafe candidate", c)
	}
}
