package cmd

import (
	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var washCmd = &cobra.Command{
	Use:     "wash [name]",
	Aliases: []string{"rm", "remove", "delete"},
	Short:   "Strip a kit and clean up (remove worktree + branch + DB + gtab)",
	Long: `wash removes a worktree's worktree dir, deletes the actual git branch,
optionally drops the DB, and removes the gtab workspace.

Pass a name to skip the picker; omit to pick from a list (or auto-resolve
from cwd when run from inside a worktree).`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()
		name, err := resolveArgOrCwdNonMaster(layout, args)
		if err != nil {
			return err
		}
		return tui.RunWashTUIFor(layout, name)
	},
}

func init() {
	rootCmd.AddCommand(washCmd)
}
