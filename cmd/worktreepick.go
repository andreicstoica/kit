package cmd

import (
	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
)

// resolveTarget walks the standard "find a worktree to act on" flow used by
// swap, warmup, links, log, and any future command that operates on one kit:
//
//  1. If args has one entry, normalize it (special-case "master").
//  2. Else, prefer the worktree the user is currently inside (cwd).
//  3. Else, open the numbered picker.
//
// Returns ("", nil) when the user aborts the picker.
func resolveTarget(layout liftoff.Layout, args []string, pickerPrompt string) (string, error) {
	if name, err := resolveArgOrCwd(layout, args); name != "" || err != nil {
		return name, err
	}
	return tui.PickWorktree(layout, pickerPrompt)
}

// resolveArgOrCwd is resolveTarget without the picker fallback. Use for
// commands whose own TUI flow has a built-in picker (play, pause, wash) so
// users don't see two pickers in a row.
//
// Returns ("", nil) when no arg and cwd is unrelated — caller's TUI takes
// over from there.
func resolveArgOrCwd(layout liftoff.Layout, args []string) (string, error) {
	return resolveArgOrCwdOpts(layout, args, false)
}

// resolveArgOrCwdNonMaster is the same as resolveArgOrCwd but ignores master
// when resolving from cwd. Use for play/pause/wash where master is never a
// valid target — the caller's TUI picker takes over instead.
func resolveArgOrCwdNonMaster(layout liftoff.Layout, args []string) (string, error) {
	return resolveArgOrCwdOpts(layout, args, true)
}

func resolveArgOrCwdOpts(layout liftoff.Layout, args []string, skipMasterCwd bool) (string, error) {
	if len(args) == 1 {
		if args[0] == "master" {
			return "master", nil
		}
		return liftoff.NormalizeAndValidate(args[0])
	}
	if n := worktreeFromCwd(layout); n != "" {
		if skipMasterCwd && n == "master" {
			return "", nil
		}
		return n, nil
	}
	return "", nil
}
