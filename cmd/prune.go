package cmd

import (
	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Bulk-wash worktrees whose PR is merged or closed",
	Long: `prune scans every worktree, flags those whose branch is merged
into master or whose PR (via gh) is MERGED/CLOSED, and lets you select which
to wash. Each selected worktree has its services stopped, dir removed,
branch deleted, slot freed, and gtab cleaned.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.RunPruneTUI(liftoff.DefaultLayout())
	},
}

func init() {
	rootCmd.AddCommand(pruneCmd)
}
