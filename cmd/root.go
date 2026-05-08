package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "0.1.0"

var rootCmd = &cobra.Command{
	Use:   "kit",
	Short: "kit — manage Liftoff feature worktrees",
	Long: `kit creates, lists, and removes isolated git-worktree feature
environments for the Liftoff app, with optional DB clones, dep installs,
graphite tracking, and ghostty workspace generation.

Subcommands borrow from soccer/fashion-kit vocabulary; classic aliases work too:
  dress (new)    create a fresh kit (worktree)
  lineup (ls)    list active kits on the field
  wash (rm)      strip and clean up a kit
  warmup (gtab)  launch the ghostty workspace for a kit
  swap (open)    open a kit in your IDE [stub]`,
	Version:       version,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
