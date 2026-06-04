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
	Short:   "Delete an old workspace",
	Long: `wash deletes a workspace folder and its code branch. It can also delete
the workspace's private database and saved Ghostty layout.

Pass a name to skip the picker; omit to pick from a list (or auto-resolve
from cwd when run from inside a workspace).

--merged switches to bulk mode: finds old workspaces whose branch
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
	washCmd.Flags().BoolVar(&washMerged, "merged", false, "bulk-delete workspaces whose branch/PR is merged or closed")
	rootCmd.AddCommand(washCmd)
}
