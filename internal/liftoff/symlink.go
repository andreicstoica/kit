package liftoff

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// nodeModulesDirs are the worktree-relative dirs where node_modules lives.
var nodeModulesDirs = []string{
	"frontend/app",
	"frontend/admin",
}

// LinkResult describes one symlink operation.
type LinkResult struct {
	Path          string
	Action        string // "linked", "skipped (no source)", "kept (already linked)", "replaced"
	StaleLockfile bool   // true if package.json/yarn.lock differ vs master
}

// LinkNodeModules symlinks the worktree's frontend node_modules dirs from
// master. Replaces existing real dirs (they were freshly created by `git
// worktree add` and are presumed empty/wrong). Already-correct symlinks are
// kept.
func LinkNodeModules(masterRepo, worktree string, onLine LineFn) ([]LinkResult, error) {
	results := make([]LinkResult, 0, len(nodeModulesDirs))
	for _, sub := range nodeModulesDirs {
		src := filepath.Join(masterRepo, sub, "node_modules")
		dst := filepath.Join(worktree, sub, "node_modules")
		res := LinkResult{Path: dst}

		if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
			res.Action = "skipped (no source — run yarn install in master first)"
			results = append(results, res)
			if onLine != nil {
				onLine(res.Action + ": " + sub)
			}
			continue
		}

		// If already a correct symlink → keep.
		if existing, err := os.Readlink(dst); err == nil {
			if existing == src {
				res.Action = "kept"
				if onLine != nil {
					onLine("kept symlink " + sub + "/node_modules")
				}
				results = append(results, res)
				continue
			}
		}

		// Remove whatever is there (real dir from `git worktree add` or stale link).
		if err := os.RemoveAll(dst); err != nil {
			return results, fmt.Errorf("remove %s: %w", dst, err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return results, err
		}
		if err := os.Symlink(src, dst); err != nil {
			return results, fmt.Errorf("symlink %s -> %s: %w", dst, src, err)
		}
		res.Action = "linked"
		res.StaleLockfile = lockfilesDiffer(masterRepo, worktree, sub)
		results = append(results, res)
		if onLine != nil {
			msg := "linked " + sub + "/node_modules"
			if res.StaleLockfile {
				msg += " (note: yarn.lock differs from master)"
			}
			onLine(msg)
		}
	}
	return results, nil
}

// lockfilesDiffer returns true if package.json or yarn.lock contents diverge
// between master and worktree for a frontend dir.
func lockfilesDiffer(masterRepo, worktree, sub string) bool {
	for _, fn := range []string{"package.json", "yarn.lock"} {
		a, err1 := os.ReadFile(filepath.Join(masterRepo, sub, fn))
		b, err2 := os.ReadFile(filepath.Join(worktree, sub, fn))
		if err1 != nil || err2 != nil {
			continue
		}
		if string(a) != string(b) {
			return true
		}
	}
	return false
}
