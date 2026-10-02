package liftoff

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// FetchMain runs `git fetch origin` from the master repo. Avoids the
// refspec form (`<main>:<main>`), which fails with exit 128 when the
// local main branch is checked out anywhere (master worktree on a
// feature branch, etc.). We branch off `origin/<main>` below, so the
// local ref doesn't need to be touched.
func (l Layout) FetchMain(onLine LineFn) error {
	args := []string{"-C", l.Master, "fetch", "origin", l.MainBranch}
	return RunStream("", "git", args, onLine)
}

// AddWorktree creates a new worktree at path with a new branch off
// origin/<main> (so it picks up the latest upstream commit even when
// the local main ref hasn't been fast-forwarded).
func (l Layout) AddWorktree(name, path string, onLine LineFn) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("worktree path already exists: %s", path)
	}
	args := []string{"-C", l.Master, "worktree", "add", path, "-b", name, "origin/" + l.MainBranch}
	return RunStream("", "git", args, onLine)
}

// RemoveWorktree removes the worktree at path (force).
func (l Layout) RemoveWorktree(path string, onLine LineFn) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		wts, err := l.ListWorktrees()
		if err != nil {
			return err
		}
		registered := false
		for _, wt := range wts {
			if filepath.Clean(wt.Path) == filepath.Clean(path) {
				registered = true
				break
			}
		}
		if !registered {
			return nil
		}
	} else if err != nil {
		return err
	}
	args := []string{"-C", l.Master, "worktree", "remove", path, "--force"}
	return RunStream("", "git", args, onLine)
}

// DeleteBranch force-deletes the local branch in the master repo.
// Non-fatal: returns nil if branch is gone.
func (l Layout) DeleteBranch(branch string, onLine LineFn) error {
	cmd := exec.Command("git", "-C", l.Master, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return nil
		}
		return fmt.Errorf("check branch %q: %w", branch, err)
	}
	args := []string{"-C", l.Master, "branch", "-D", branch}
	return RunStream("", "git", args, onLine)
}

// MasterIsRepo returns true if the master path exists and looks like a git repo.
func (l Layout) MasterIsRepo() bool {
	if _, err := os.Stat(l.Master); err != nil {
		return false
	}
	if _, err := os.Stat(l.Master + "/.git"); err != nil {
		return false
	}
	return true
}
