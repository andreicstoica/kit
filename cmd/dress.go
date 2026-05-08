package cmd

import (
	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var dressCmd = &cobra.Command{
	Use:     "dress",
	Aliases: []string{"new"},
	Short:   "Put on a fresh kit (create a new feature worktree)",
	Long: `dress walks you through creating a new Liftoff feature worktree:

  - git worktree off latest origin/master
  - copies the four .env files
  - optionally clones the local DB into liftoff_<name>
  - optionally installs backend (pip) + frontend (yarn) deps
  - optionally registers the branch with graphite (gt track)
  - optionally writes a gtab AppleScript workspace

A leading "liftoff-" in the name is stripped automatically. Worktrees land at
~/liftoff/<name> with branch <name>.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.RunDressTUI(liftoff.DefaultLayout())
	},
}

func init() {
	rootCmd.AddCommand(dressCmd)
}
