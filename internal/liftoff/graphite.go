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

// StackEntry is one row from `gt ls -s` parsed into its parts.
type StackEntry struct {
	Glyph   string // ◉ for current, ◯ otherwise
	Branch  string // branch name
	Hint    string // trailing parenthetical (eg "(needs restack)"), may be ""
	Current bool   // true when this is the currently-checked-out branch
}

// GtStackOf returns the gt-tracked stack containing the branch currently
// checked out in worktreePath: trunk first, current branch last. Returns
// nil if the branch is untracked or `gt` isn't installed.
//
// Uses `gt ls -s` (the `gt log short --stack` alias) which already scopes
// the output to this worktree's chain.
func (l Layout) GtStackOf(worktreePath string) []StackEntry {
	if !HasGraphite() {
		return nil
	}
	cmd := exec.Command("gt", "ls", "-s")
	cmd.Dir = worktreePath
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var stack []StackEntry
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			continue
		}
		// Split on the first whitespace gap: "glyph  branch + hints".
		// gt uses two spaces between the glyph column and the name.
		fields := strings.SplitN(strings.TrimLeft(line, " "), "  ", 2)
		if len(fields) != 2 {
			continue
		}
		glyph := strings.TrimSpace(fields[0])
		rest := strings.TrimSpace(fields[1])
		if rest == "" {
			continue
		}
		entry := StackEntry{Glyph: glyph, Current: strings.Contains(glyph, "◉")}
		if p := strings.Index(rest, " ("); p >= 0 {
			entry.Branch = rest[:p]
			entry.Hint = rest[p+1:] // drop leading space, keep "(...)"
		} else {
			entry.Branch = rest
		}
		stack = append(stack, entry)
	}
	// gt ls -s outputs current branch first, trunk last. Reverse so trunk is first.
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
