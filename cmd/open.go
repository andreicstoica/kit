package cmd

import (
	"fmt"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/spf13/cobra"
)

var openLayout string

var openCmd = &cobra.Command{
	Use:               "open [name]",
	Short:             "Open a worktree's persistent Herdr space",
	Long:              "open creates the worktree's Herdr space and Kit-owned tabs once, then attaches to the shared Herdr session. On first open, Kit offers the familiar Simple (2 tabs) or Detailed (5 tabs) layout picker; existing spaces reconnect without prompting.",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()
		name, err := resolveTarget(layout, args, "kit open — pick a worktree")
		if err != nil || name == "" {
			return err
		}
		path, err := layout.ResolveWorktreePath(name)
		if err != nil {
			return err
		}
		chosenLayout, err := resolveHerdrLayout(name, path, openLayout)
		if err != nil {
			return err
		}
		workspace, err := liftoff.OpenHerdr(name, path, chosenLayout)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "opened %s in Herdr (%s)\n", name, workspace.WorkspaceID)
		return liftoff.AttachHerdr()
	},
}

func init() {
	openCmd.Flags().StringVar(&openLayout, "layout", "", "Kit Herdr layout (default, detailed, ai, or a configured layout)")
	rootCmd.AddCommand(openCmd)
}
