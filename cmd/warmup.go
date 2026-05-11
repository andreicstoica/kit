package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/spf13/cobra"
)

var warmupCmd = &cobra.Command{
	Use:     "warmup [name]",
	Aliases: []string{"gtab"},
	Short:   "Pre-match warmup: launch the gtab ghostty workspace for a kit",
	Long: "**warmup** opens the kit's pre-built ghostty workspace (4 tabs " +
		"laid out for frontend + backend + celery + scratch).\n\n" +
		"With no arg, uses the worktree you're in (or the numbered picker if " +
		"cwd is unrelated or master).",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()
		name, err := resolveTarget(layout, args, "kit warmup — pick a kit")
		if err != nil {
			return err
		}
		if name == "" {
			return nil
		}
		if name == "master" {
			// No per-feature gtab template; open Ghostty rooted at master.
			c := exec.Command("open", "-a", "Ghostty.app", layout.Master)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Start()
		}
		if !layout.HasGtab(name) {
			return fmt.Errorf("no gtab workspace at %s — re-run `kit design` or write one manually", layout.GtabFile(name))
		}
		return layout.LaunchGtab(name)
	},
}

func init() {
	rootCmd.AddCommand(warmupCmd)
}
