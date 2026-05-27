package cmd

import (
	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var washMerged bool

var washCmd = &cobra.Command{
	Use:     "wash [name]",
	Aliases: []string{"rm", "remove", "delete"},
	Short:   "Strip a kit and clean up (remove worktree + branch + DB + gtab)",
	Long: `wash removes a worktree's worktree dir, deletes the actual git branch,
optionally drops the DB, and removes the gtab workspace.

Pass a name to skip the picker; omit to pick from a list (or auto-resolve
from cwd when run from inside a worktree).

--merged switches to bulk mode: scans every worktree, flags any whose branch
is merged into master or whose PR (via gh) is MERGED/CLOSED, and lets you
multi-select which to wash.`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()
		if washMerged {
			return tui.RunMergedWashTUI(layout)
		}
		name, err := resolveArgOrCwdNoMaster(layout, args)
		if err != nil {
			return err
		}
		return tui.RunWashTUIFor(layout, name)
	},
}

func init() {
	washCmd.Flags().BoolVar(&washMerged, "merged", false, "bulk-wash worktrees whose branch/PR is merged or closed")
	rootCmd.AddCommand(washCmd)
}
