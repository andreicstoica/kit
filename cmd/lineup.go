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
	Short:   "Show the kits currently available",
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()
		if !layout.MasterIsRepo() {
			return fmt.Errorf("master repo not found at %s (set KIT_ROOT/KIT_MASTER_DIR)", layout.Master)
		}
		var (
			out string
			err error
		)
		if lineupTree {
			out, err = tui.RenderLineupTree(layout)
		} else {
			out, err = tui.RenderLineup(layout)
		}
		if err != nil {
			return err
		}
		fmt.Print(out)
		return nil
	},
}

func init() {
	lineupCmd.Flags().BoolVarP(&lineupTree, "tree", "t", false, "render as a tree (master → worktrees → running services)")
	rootCmd.AddCommand(lineupCmd)
}
