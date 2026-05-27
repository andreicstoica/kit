package cmd

import (
	"fmt"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var lineupTree bool

var lineupCmd = &cobra.Command{
	Use:     "lineup",
	Aliases: []string{"ls", "list"},
	Short:   "Show the kits currently available (--tree for the tree view)",
	Long: "**lineup** lists every kit. Default is a table; `--tree` renders the " +
		"same set as a tree rooted at master, expanding each worktree's gt " +
		"stack, setup signals (db ownership + node_modules wiring), and " +
		"running services.",
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()
		if !layout.MasterIsRepo() {
			return fmt.Errorf("master repo not found at %s (set KIT_ROOT/KIT_MASTER_DIR)", layout.Master)
		}
		render := tui.RenderLineup
		if lineupTree {
			render = tui.RenderLineupTree
		}
		out, err := render(layout)
		if err != nil {
			return err
		}
		fmt.Print(out)
		return nil
	},
}

func init() {
	lineupCmd.Flags().BoolVar(&lineupTree, "tree", false, "render as a tree (master → worktrees → stack/setup/services)")
	rootCmd.AddCommand(lineupCmd)
}
