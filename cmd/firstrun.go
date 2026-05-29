package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

// commandsThatBypassFirstRun never trigger the "you haven't run setup"
// prompt — running setup itself, asking for help/version, or running
// diagnostics shouldn't loop back to a setup prompt.
var commandsThatBypassFirstRun = map[string]bool{
	"setup":      true,
	"doctor":     true,
	"physio":     true,
	"help":       true,
	"completion": true,
	"man":        true,
	"guide":      true,
}

// MaybeOfferSetup runs before every other command. If the kit config dir
// doesn't exist yet AND the current command isn't one of the exempt
// onboarding commands, prompt the user to run `kit setup`.
//
// Wired via PersistentPreRunE on rootCmd (registered in root.go init()).
func MaybeOfferSetup(cmd *cobra.Command, args []string) error {
	if commandsThatBypassFirstRun[cmd.Name()] {
		return nil
	}
	for p := cmd.Parent(); p != nil; p = p.Parent() {
		if commandsThatBypassFirstRun[p.Name()] {
			return nil
		}
	}
	home, _ := os.UserHomeDir()
	configDir := filepath.Join(home, ".config", "kit")
	if _, err := os.Stat(configDir); err == nil {
		return nil
	}
	fmt.Println(tui.StyleWarn.Render("kit hasn't been set up on this machine yet."))
	fmt.Println(tui.StyleDim.Render("`kit setup` installs missing tools, clones master, and adopts existing worktrees."))
	fmt.Println()

	run, err := tui.RunConfirm(tui.ConfirmConfig{
		Title:    "Run `kit setup` now?",
		Negative: "Cancel",
		Default:  true,
	})
	if err != nil {
		return err
	}
	if !run {
		return fmt.Errorf("setup required — run `kit setup` when ready")
	}
	return runSetup(cmd, nil)
}
