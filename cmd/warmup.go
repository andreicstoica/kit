package cmd

import (
	"fmt"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var warmupCmd = &cobra.Command{
	Use:     "warmup [name]",
	Aliases: []string{"gtab"},
	Short:   "Pre-match warmup: launch the gtab ghostty workspace for a kit",
	Long: "**warmup** opens the kit's pre-built ghostty workspace (4 tabs " +
		"laid out for frontend + backend + celery + scratch).\n\n" +
		"With no arg, uses the worktree you're in (or a picker if cwd is " +
		"unrelated). Numeric quick-pick (1-9) is supported in the picker.",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()

		name, err := resolveWarmupTarget(layout, args)
		if err != nil {
			return err
		}
		if name == "" {
			return nil // user aborted picker
		}
		if name == "master" {
			return fmt.Errorf("no gtab workspace for master — warmup is per-feature")
		}
		if !layout.HasGtab(name) {
			return fmt.Errorf("no gtab workspace at %s — re-run `kit design` or write one manually", layout.GtabFile(name))
		}
		return layout.LaunchGtab(name)
	},
}

func resolveWarmupTarget(layout liftoff.Layout, args []string) (string, error) {
	if len(args) == 1 {
		return liftoff.NormalizeAndValidate(args[0])
	}
	if n := worktreeFromCwd(layout); n != "" && n != "master" {
		return n, nil
	}
	return tui.PickWorktree(layout, "kit warmup — pick a kit")
}

func init() {
	rootCmd.AddCommand(warmupCmd)
}
