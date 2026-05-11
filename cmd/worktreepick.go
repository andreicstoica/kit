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
	if len(args) == 1 {
		if args[0] == "master" {
			return "master", nil
		}
		return liftoff.NormalizeAndValidate(args[0])
	}
	if n := worktreeFromCwd(layout); n != "" {
		return n, nil
	}
	return "", nil
}
