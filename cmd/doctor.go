package cmd

import (
	"fmt"
	"os"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:     "doctor",
	Aliases: []string{"physio"},
	Short:   "Diagnose your kit setup",
	Long: "**doctor** runs a series of read-only checks to confirm your toolchain " +
		"is ready for `kit`.\n\nChecks:\n\n" +
		"- Homebrew (and whether brew is on PATH)\n" +
		"- git, gh (with auth), node + yarn, python (with venv)\n" +
		"- redis, postgres (running on default ports)\n" +
		"- Ghostty, an editor (Zed / Cursor / VS Code)\n" +
		"- The Liftoff master repo and its node_modules\n\n" +
		"Exits non-zero on any failure. Pair with `kit setup` to fix what's missing.",
	RunE: func(cmd *cobra.Command, args []string) error {
		results := liftoff.RunChecks(liftoff.DefaultChecks(liftoff.DefaultLayout()))
		fmt.Print(tui.RenderDoctor(results))
		if liftoff.AnyFailed(results) {
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
