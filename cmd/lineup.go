package cmd

import (
	"fmt"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var (
	lineupTree   bool
	lineupRemote bool
)

var lineupCmd = &cobra.Command{
	Use:     "lineup",
	Aliases: []string{"ls", "list"},
	Short:   "Show the kits currently available (--tree for the tree view)",
	Long: "**lineup** lists every kit. Default is a table with Herdr space/agent " +
		"status when Herdr is available; `--tree` renders the " +
		"same set as a tree rooted at master, expanding each worktree's gt " +
		"stack, setup signals (db ownership + node_modules wiring), and " +
		"running services. Use `--remote` to add fresh GitHub PR status, " +
		"including saved records with missing worktrees.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		tui.DetectTerminalBackground()
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
		fmt.Fprint(cmd.OutOrStdout(), out)
		if lineupRemote {
			return printRemotePRs(cmd.OutOrStdout(), layout)
		}
		return nil
	},
}

func init() {
	lineupCmd.Flags().BoolVar(&lineupTree, "tree", false, "render as a tree (master → worktrees → stack/setup/services)")
	lineupCmd.Flags().BoolVar(&lineupRemote, "remote", false, "show fresh GitHub PR status, including saved records with missing worktrees")
	rootCmd.AddCommand(lineupCmd)
}
