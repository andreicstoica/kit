package cmd

import (
	"fmt"
	"os"

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
		// Auto-write the gtab AppleScript if it's missing. Lets `kit warmup
		// master` (or any adopted worktree that was never designed) work
		// without forcing the user to re-run `kit design`.
		if !layout.HasGtab(name) {
			path := layout.Master
			if name != "master" {
				path = layout.WorktreePath(name)
				if _, err := os.Stat(path); err != nil {
					legacy := layout.LegacyWorktreePath(name)
					if _, err2 := os.Stat(legacy); err2 == nil {
						path = legacy
					} else {
						return fmt.Errorf("worktree path missing: %s", path)
					}
				}
			}
			if _, err := layout.WriteGtab(name, path); err != nil {
				return fmt.Errorf("write gtab: %w", err)
			}
		}
		return layout.LaunchGtab(name)
	},
}

func init() {
	rootCmd.AddCommand(warmupCmd)
}
