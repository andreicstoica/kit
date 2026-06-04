package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var (
	syncSkipTear    bool
	syncSkipMigrate bool
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Run `gt sync` in master, then offer to wash merged/closed worktrees",
	Long: "**sync** is the daily refresh:\n\n" +
		"1. Runs `gt sync` inside the master repo (pulls trunk, restacks, prunes merged local branches).\n" +
		"2. If `gt sync` fast-forwarded master onto new Alembic migrations, runs `alembic upgrade head` in master's backend so the local master DB mirrors remote master.\n" +
		"3. Scans worktrees for merged/closed PR branches; if any remain, prompts to run `kit wash --merged` (multi-select wash).\n\n" +
		"Requires `gt` (Graphite). Pass `--no-tear` to skip the post-sync merged-wash, `--no-migrate` to skip the Alembic upgrade.",
	RunE: runSync,
}

func init() {
	syncCmd.Flags().BoolVar(&syncSkipTear, "no-tear", false, "skip the merged-wash prompt after `gt sync`")
	syncCmd.Flags().BoolVar(&syncSkipMigrate, "no-migrate", false, "skip the `alembic upgrade head` after `gt sync`")
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
	// Snapshot master's HEAD so we can tell, post-sync, whether it
	// fast-forwarded onto new migrations. Best-effort: an error here just
	// disables the migration check (NewMigrations treats "" as no-op).
	oldHead, _ := layout.MasterHead()

	gt := exec.Command("gt", "sync")
	gt.Dir = layout.Master
	gt.Stdin = os.Stdin
	gt.Stdout = os.Stdout
	gt.Stderr = os.Stderr
	if err := gt.Run(); err != nil {
		return fmt.Errorf("gt sync failed: %w", err)
	}

	if !syncSkipMigrate {
		if err := runMasterMigrate(layout, oldHead); err != nil {
			return err
		}
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

// runMasterMigrate runs `alembic upgrade head` in master's backend when
// `gt sync` fast-forwarded master onto new Alembic version files. No-op when
// master didn't move or no migrations landed — so the local master DB always
// mirrors remote master, staying a clean base for worktrees to clone from.
func runMasterMigrate(layout liftoff.Layout, oldHead string) error {
	newHead, err := layout.MasterHead()
	if err != nil {
		return nil // can't compare; skip rather than block the sync
	}
	migs, err := layout.NewMigrations(oldHead, newHead)
	if err != nil || len(migs) == 0 {
		return nil
	}

	fmt.Println()
	fmt.Println(tui.StyleTitle.Render(fmt.Sprintf("%d new migration(s) on master — alembic upgrade head", len(migs))))
	for _, m := range migs {
		fmt.Printf("  %s\n", tui.StyleDim.Render(m))
	}
	if err := layout.AlembicUpgradeHead(func(line string) {
		fmt.Println("  " + tui.StyleDim.Render(line))
	}); err != nil {
		// Don't abort the sync on a failed upgrade — surface it and let the
		// merged-wash flow continue.
		fmt.Println(tui.StyleErr.Render("✗ alembic upgrade failed: " + err.Error()))
		return nil
	}
	fmt.Println(tui.StyleOK.Render("✓ master DB upgraded to head."))
	return nil
}
