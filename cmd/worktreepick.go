package cmd

import (
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
	return resolveArgOrCwdOpts(layout, args, false)
}

// resolveArgOrCwdNonMaster ignores master from cwd. For play/pause/wash
// where master is never a valid target.
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
