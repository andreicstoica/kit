package cmd

import (
	"fmt"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/spf13/cobra"
)

var closeCmd = &cobra.Command{
	Use:               "close [name]",
	Short:             "Explicitly delete a worktree's Herdr space",
	Long:              "close removes the Herdr workspace, tabs, panes, and agents for a worktree but leaves the Git worktree and Kit services intact. The next `kit open` recreates the space.",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()
		name, err := resolveTarget(layout, args, "kit close — pick a worktree")
		if err != nil || name == "" {
			return err
		}
		path, err := layout.ResolveWorktreePath(name)
		if err != nil {
			return err
		}
		if err := liftoff.CloseHerdr(name, path); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "closed Herdr space for %s\n", name)
		return nil
	},
}

func init() { rootCmd.AddCommand(closeCmd) }
