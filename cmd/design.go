package cmd

import (
	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var designCmd = &cobra.Command{
	Use:     "design [name]",
	Aliases: []string{"new"},
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
		"Passing `name` pre-fills the wizard's first field. A leading `liftoff-`\n" +
		"is stripped from your input. Worktrees land at `~/liftoff/<name>` with\n" +
		"branch `<name>`.\n\n" +
		"Alias: `new` (muscle-memory).",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		prefill := ""
		if len(args) == 1 {
			prefill = args[0]
		}
		return tui.RunDesignTUI(liftoff.DefaultLayout(), prefill)
	},
}

func init() {
	rootCmd.AddCommand(designCmd)
}
