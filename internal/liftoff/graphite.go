package liftoff

import "os/exec"

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
