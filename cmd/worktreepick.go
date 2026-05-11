package cmd

import (
	"fmt"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
)

// resolveTarget resolves arg → cwd → picker. Returns ("", nil) on abort.
func resolveTarget(layout liftoff.Layout, args []string, pickerPrompt string) (string, error) {
	if name, err := resolveArgOrCwd(layout, args); name != "" || err != nil {
		return name, err
	}
	return tui.PickWorktree(layout, pickerPrompt)
}

// resolveArgOrCwd is resolveTarget minus the picker — for commands whose
// own TUI has a built-in picker (avoids two pickers in a row).
func resolveArgOrCwd(layout liftoff.Layout, args []string) (string, error) {
	return resolveArgOrCwdOpts(layout, args, false, false)
}

// resolveArgOrCwdSkipMasterCwd ignores master when picked up from cwd
// (still accepts an explicit `master` arg). Use for play/pause so
// running from the master dir falls through to the picker, but
// `kit play master` is honored.
func resolveArgOrCwdSkipMasterCwd(layout liftoff.Layout, args []string) (string, error) {
	return resolveArgOrCwdOpts(layout, args, true, false)
}

// resolveArgOrCwdNoMaster rejects master from both cwd and arg. For
// wash where master would mean deleting the main repo.
func resolveArgOrCwdNoMaster(layout liftoff.Layout, args []string) (string, error) {
	return resolveArgOrCwdOpts(layout, args, true, true)
}

func resolveArgOrCwdOpts(layout liftoff.Layout, args []string, skipMasterCwd, rejectMasterArg bool) (string, error) {
	if len(args) == 1 {
		if args[0] == "master" {
			if rejectMasterArg {
				return "", fmt.Errorf("master isn't a kit — pick a worktree instead")
			}
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
