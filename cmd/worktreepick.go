package cmd

import (
	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
)

// resolveTarget walks the standard "find a worktree to act on" flow used by
// swap, warmup, and any future command that operates on one kit:
//
//  1. If args has one entry, normalize it (special-case "master").
//  2. Else, prefer the worktree the user is currently inside (cwd).
//     When skipMaster is true, the cwd fallback only applies to feature
//     worktrees — master falls through to the picker.
//  3. Else, open the numbered picker.
//
// Returns ("", nil) when the user aborts the picker.
func resolveTarget(layout liftoff.Layout, args []string, pickerPrompt string, skipMaster bool) (string, error) {
	if len(args) == 1 {
		if args[0] == "master" {
			return "master", nil
		}
		return liftoff.NormalizeAndValidate(args[0])
	}
	if n := worktreeFromCwd(layout); n != "" {
		if !(skipMaster && n == "master") {
			return n, nil
		}
	}
	return tui.PickWorktree(layout, pickerPrompt)
}
