package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var syncSkipTear bool

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Run `gt sync` in master, then offer to wash merged/closed worktrees",
	Long: "**sync** is the daily refresh:\n\n" +
		"1. Runs `gt sync` inside the master repo (pulls trunk, restacks, prunes merged local branches).\n" +
		"2. Scans worktrees for merged/closed PR branches; if any remain, prompts to run `kit wash --merged` (multi-select wash).\n\n" +
		"Requires `gt` (Graphite). Pass `--no-tear` to skip the post-sync merged-wash.",
	RunE: runSync,
}

func init() {
	syncCmd.Flags().BoolVar(&syncSkipTear, "no-tear", false, "skip the merged-wash prompt after `gt sync`")
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	if !liftoff.HasGraphite() {
		return fmt.Errorf("gt not installed — run `kit setup` or `brew install withgraphite/tap/graphite`")
	}
	layout := liftoff.DefaultLayout()
	if !layout.MasterIsRepo() {
		return fmt.Errorf("master repo not found at %s", layout.Master)
	}

	fmt.Println(tui.StyleTitle.Render("kit sync — gt sync in master"))
	gt := exec.Command("gt", "sync")
	gt.Dir = layout.Master
	gt.Stdin = os.Stdin
	gt.Stdout = os.Stdout
	gt.Stderr = os.Stderr
	if err := gt.Run(); err != nil {
		return fmt.Errorf("gt sync failed: %w", err)
	}

	if syncSkipTear {
		return nil
	}

	cands, err := layout.FindMergedWorktrees()
	if err != nil {
		return err
	}
	if len(cands) == 0 {
		fmt.Println()
		fmt.Println(tui.StyleOK.Render("✓ no merged worktrees to wash."))
		return nil
	}

	fmt.Println()
	fmt.Println(tui.StyleTitle.Render(fmt.Sprintf("found %d merged/closed worktree(s)", len(cands))))
	for _, c := range cands {
		fmt.Printf("  %s  %s\n", c.Name, tui.StyleDim.Render("("+c.Reason+")"))
	}
	fmt.Println()

	accept, err := tui.RunConfirm(tui.ConfirmConfig{
		Title:    "Run `kit wash --merged` to wash them?",
		Negative: "Skip",
		Default:  true,
	})
	if err != nil {
		return err
	}
	if !accept {
		return nil
	}
	return tui.RunMergedWashTUI(layout)
}
