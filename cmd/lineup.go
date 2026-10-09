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
	lineupAgents bool
	lineupParked bool
)

var lineupCmd = &cobra.Command{
	Use:     "lineup",
	Aliases: []string{"ls", "list"},
	Short:   "Show the kits currently available (--tree for the tree view)",
	Long: "**lineup** lists every kit. Default is a table with the selected terminal's " +
		"workspace status. `--agents` shows OSC 7501 records across all Rex sessions. `--tree` renders the " +
		"same set as a tree rooted at master, expanding each worktree's gt " +
		"stack, setup signals (db ownership + node_modules wiring), and " +
		"running services. Use `--remote` to add fresh GitHub PR status, " +
		"including saved records with missing worktrees. Parked workspaces are hidden; " +
		"use `--parked` to show only parked workspaces.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		tui.DetectTerminalBackground()
		if lineupAgents {
			state, err := liftoff.ReadRexState()
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), tui.RenderRexProgramStatus(state))
			return nil
		}
		layout := liftoff.DefaultLayout()
		if !layout.MasterIsRepo() {
			return fmt.Errorf("master repo not found at %s (set KIT_ROOT/KIT_MASTER_DIR)", layout.Master)
		}
		render := tui.RenderLineup
		if lineupTree {
			render = tui.RenderLineupTree
		}
		if lineupParked {
			render = tui.RenderLineupParked
			if lineupTree {
				render = tui.RenderLineupTreeParked
			}
		}
		out, err := render(layout)
		if err != nil {
			return err
		}
		fmt.Fprint(cmd.OutOrStdout(), out)
		if lineupRemote {
			return printRemotePRsForVisibility(cmd.OutOrStdout(), layout, lineupParked)
		}
		return nil
	},
}

func init() {
	lineupCmd.Flags().BoolVar(&lineupParked, "parked", false, "show only parked workspaces (hidden by default)")
	lineupCmd.Flags().BoolVar(&lineupAgents, "agents", false, "show OSC 7501 program status across all Rex sessions, including unmapped workspaces")
	lineupCmd.Flags().BoolVar(&lineupTree, "tree", false, "render as a tree (master → worktrees → stack/setup/services)")
	lineupCmd.Flags().BoolVar(&lineupRemote, "remote", false, "show fresh GitHub PR status, including saved records with missing worktrees")
	lineupCmd.MarkFlagsMutuallyExclusive("agents", "tree")
	lineupCmd.MarkFlagsMutuallyExclusive("agents", "remote")
	lineupCmd.MarkFlagsMutuallyExclusive("agents", "parked")
	rootCmd.AddCommand(lineupCmd)
}
