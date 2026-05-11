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

// GtStackOf returns the gt-tracked stack containing the branch currently
// checked out in worktreePath: trunk first, current branch last. Returns
// nil if the branch is untracked or `gt` isn't installed.
//
// Uses `gt ls -s` (the `gt log short --stack` alias) which already scopes
// the output to this worktree's chain.
func (l Layout) GtStackOf(worktreePath string) []string {
	if !HasGraphite() {
		return nil
	}
	cmd := exec.Command("gt", "ls", "-s")
	cmd.Dir = worktreePath
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	// Lines look like "◉  branch-name (needs restack)" or "◯  branch-name (other-worktree)".
	// First column is a glyph, then branch name, then optional parenthetical tags.
	var stack []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Drop leading glyph chars (anything before the first ASCII letter or digit).
		i := 0
		for ; i < len(line); i++ {
			c := line[i]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
				break
			}
		}
		rest := strings.TrimSpace(line[i:])
		if rest == "" {
			continue
		}
		// Keep "(needs restack)" / "(liftoff-X)" suffixes — they're useful
		// signal in the per-branch stack display.
		stack = append(stack, rest)
	}
	// gt ls -s outputs current branch first, trunk last. Reverse so trunk is first
	// (matches the "master → branch" left-to-right reading order).
	for i, j := 0, len(stack)-1; i < j; i, j = i+1, j-1 {
		stack[i], stack[j] = stack[j], stack[i]
	}
	return stack
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
