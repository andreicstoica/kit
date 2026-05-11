package cmd

import (
	"fmt"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var formationCmd = &cobra.Command{
	Use:     "formation",
	Aliases: []string{"tree"},
	Short:   "Show the kits as a tree (master → worktrees → stack/setup/services)",
	Long: "**formation** renders the same kits `kit lineup` shows, but as a " +
		"tree rooted at master. Each worktree expands its gt stack, setup " +
		"signals (db ownership + node_modules wiring), and running services.\n\n" +
		"Aliases: `tree`.",
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()
		if !layout.MasterIsRepo() {
			return fmt.Errorf("master repo not found at %s (set KIT_ROOT/KIT_MASTER_DIR)", layout.Master)
		}
		out, err := tui.RenderLineupTree(layout)
		if err != nil {
			return err
		}
		fmt.Print(out)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(formationCmd)
}
