package cmd

import (
	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var designCmd = &cobra.Command{
	Use:     "design",
	Aliases: []string{"dress", "new"},
	Short:   "Design a fresh kit (create a new feature worktree)",
	Long: "**design** walks you through creating a new Liftoff feature worktree:\n\n" +
		"- `git fetch origin master:master` then `git worktree add ~/liftoff/<name> -b <name> master`\n" +
		"- copies the four `.env` files\n" +
		"- (opt) clones local DB into `liftoff_<name>` and rewrites `SQLALCHEMY_DATABASE_NAME`\n" +
		"- (opt) installs backend deps with pip\n" +
		"- (opt) symlinks frontend node_modules from master (saves ~2GB + skips yarn install)\n" +
		"- (opt) registers the branch with graphite (`gt track`)\n" +
		"- writes a gtab AppleScript workspace\n" +
		"- allocates a port slot (recorded in `~/.config/kit/state.toml`)\n\n" +
		"A leading `liftoff-` is stripped from your input. Worktrees land at\n" +
		"`~/liftoff/<name>` with branch `<name>`.\n\n" +
		"Aliases: `dress`, `new` (muscle-memory).",
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.RunDesignTUI(liftoff.DefaultLayout())
	},
}

func init() {
	rootCmd.AddCommand(designCmd)
}
