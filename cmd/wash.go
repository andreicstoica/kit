package cmd

import (
	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var washCmd = &cobra.Command{
	Use:     "wash",
	Aliases: []string{"rm", "remove"},
	Short:   "Strip a kit and clean up (remove worktree + branch + DB + gtab)",
	Long: `wash opens a picker of active kits. Select one to remove its worktree,
delete the branch, optionally drop the DB, and remove the gtab workspace.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.RunWashTUI(liftoff.DefaultLayout())
	},
}

func init() {
	rootCmd.AddCommand(washCmd)
}
