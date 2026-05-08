package cmd

import (
	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var tearCmd = &cobra.Command{
	Use:     "tear",
	Aliases: []string{"prune"},
	Short:   "Tear up the field — bulk-wash merged/closed worktrees",
	Long: "**tear** scans every worktree, flags any whose branch is merged into master or whose PR (via `gh`) is `MERGED`/`CLOSED`, and lets you multi-select which to wash.\n\n" +
		"Each selected worktree has its:\n\n" +
		"- services stopped\n" +
		"- dir removed (`git worktree remove --force`)\n" +
		"- branch deleted\n" +
		"- port slot freed\n" +
		"- gtab AppleScript removed\n\n" +
		"Alias: `prune`.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.RunPruneTUI(liftoff.DefaultLayout())
	},
}

func init() {
	rootCmd.AddCommand(tearCmd)
}
