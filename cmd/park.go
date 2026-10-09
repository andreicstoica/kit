package cmd

import (
	"fmt"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/spf13/cobra"
)

var parkCmd = &cobra.Command{
	Use:               "park <name>",
	Short:             "Stop services and hide a workspace without deleting its resources",
	Long:              "Park stops Kit-managed services and hides the workspace from default lineup. Files, commits, branches, databases, port slots, and terminal panes are kept. Running agents are not stopped.",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := liftoff.DefaultLayout().Park(args[0]); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "parked %s; files, branch, DB, and terminal panes kept\n", args[0])
		return nil
	},
}

var resumeCmd = &cobra.Command{
	Use:               "resume <name>",
	Short:             "Show a parked workspace again without starting services",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := liftoff.DefaultLayout().Resume(args[0]); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "resumed %s; use kit play %s to start services\n", args[0], args[0])
		return nil
	},
}

func init() {
	rootCmd.AddCommand(parkCmd, resumeCmd)
}
