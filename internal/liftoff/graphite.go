package liftoff

import (
	"os/exec"
	"strings"
)

// HasGraphite returns true if `gt` is on PATH.
func HasGraphite() bool {
	_, err := exec.LookPath("gt")
	return err == nil
}

// GtTrack runs `gt track --parent <main>` inside the worktree.
// Registers the new branch with graphite so it shows up in stacks
// and is ready for `gt submit` later.
func (l Layout) GtTrack(worktree string, onLine LineFn) error {
	return RunStream(worktree, "gt", []string{"track", "--parent", l.MainBranch}, onLine)
}

// GtParentOf returns the Graphite-tracked parent branch for the branch
// currently checked out in worktreePath. Returns "" if the branch is
// untracked (or `gt` isn't installed). Non-zero exit from `gt parent`
// is the untracked signal.
func (l Layout) GtParentOf(worktreePath string) string {
	if !HasGraphite() {
		return ""
	}
	cmd := exec.Command("gt", "parent")
	cmd.Dir = worktreePath
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// NeedsRestack reports whether the branch in worktreePath needs to be
// rebased on top of its parent. True when parent's HEAD is NOT an
// ancestor of the worktree's HEAD (parent has moved forward since this
// branch was last restacked).
//
// parent should be the graphite parent branch name. Returns false if
// parent is empty.
func (l Layout) NeedsRestack(worktreePath, parent string) bool {
	if parent == "" {
		return false
	}
	cmd := exec.Command("git", "merge-base", "--is-ancestor", parent, "HEAD")
	cmd.Dir = worktreePath
	if err := cmd.Run(); err != nil {
		return true
	}
	return false
}
