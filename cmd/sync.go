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
		"2. When master is on the trunk branch, checks the liftoff DB against Alembic head and runs `alembic upgrade head` if migrations are pending.\n" +
		"3. Scans worktrees for merged/closed PR branches; if any remain, prompts to run `kit wash --merged` (multi-select wash).\n\n" +
		"Requires `gt` (Graphite). Pass `--no-tear` to skip the post-sync merged-wash, `--no-migrate` to skip the Alembic upgrade.",
	RunE: runSync,
}

func init() {
	syncCmd.Flags().BoolVar(&syncSkipTear, "no-tear", false, "skip the merged-wash prompt after `gt sync`")
	syncCmd.Flags().BoolVar(&syncSkipMigrate, "no-migrate", false, "skip the `alembic upgrade head` after `gt sync`")
	rootCmd.AddCommand(syncCmd)
}

type migrateStatus int

const (
	migrateSkippedNoMigrate migrateStatus = iota
	migrateSkippedNotTrunk
	migrateAtHead
	migrateUpgraded
	migrateFailed
)

type migrateResult struct {
	status migrateStatus
	branch string
	err    error
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
	// Snapshot master's HEAD so we can list migrations that landed during this
	// sync. Best-effort: an error here just disables the listing. Only snapshot
	// when the master repo is actually on the configured trunk branch.
	oldHead := ""
	if branch, err := layout.MasterBranch(); err == nil && branch == layout.MainBranch {
		oldHead, _ = layout.MasterHead()
	}

	gt := exec.Command("gt", "sync")
	gt.Dir = layout.Master
	gt.Stdin = os.Stdin
	gt.Stdout = os.Stdout
	gt.Stderr = os.Stderr
	if err := gt.Run(); err != nil {
		return fmt.Errorf("gt sync failed: %w", err)
	}

	migrate := migrateResult{status: migrateSkippedNoMigrate}
	if !syncSkipMigrate {
		migrate = runMasterMigrate(layout, oldHead)
	}

	var retErr error
	if syncSkipTear {
		printMigrateSummary(layout, migrate)
		return retErr
	}

	cands, err := layout.FindMergedWorktrees()
	if err != nil {
		return err
	}
	if len(cands) == 0 {
		printMigrateSummary(layout, migrate)
		fmt.Println(tui.StyleOK.Render("✓ no merged worktrees to wash."))
		return retErr
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
		printMigrateSummary(layout, migrate)
		return retErr
	}
	retErr = tui.RunMergedWashTUI(layout)
	printMigrateSummary(layout, migrate)
	return retErr
}

func printMigrateSummary(layout liftoff.Layout, migrate migrateResult) {
	fmt.Println()
	switch migrate.status {
	case migrateSkippedNoMigrate:
		fmt.Println(tui.StyleDim.Render("⊘ master DB migration skipped (--no-migrate)."))
	case migrateSkippedNotTrunk:
		fmt.Println(tui.StyleDim.Render(fmt.Sprintf(
			"⊘ master DB migration skipped (master on %s, want %s).",
			migrate.branch, layout.MainBranch,
		)))
	case migrateAtHead:
		fmt.Println(tui.StyleOK.Render("✓ master DB (liftoff) already at head."))
	case migrateUpgraded:
		fmt.Println(tui.StyleOK.Render("✓ master DB (liftoff) upgraded to head."))
	case migrateFailed:
		if migrate.err != nil {
			fmt.Println(tui.StyleErr.Render("✗ master DB migration failed: " + migrate.err.Error()))
		} else {
			fmt.Println(tui.StyleErr.Render("✗ master DB migration failed."))
		}
	}
	if migrate.status != migrateSkippedNoMigrate && migrate.status != migrateSkippedNotTrunk {
		fmt.Println(tui.StyleDim.Render("  feature DBs were not migrated."))
	}
}

// runMasterMigrate runs `alembic upgrade head` in master's backend when the
// liftoff DB is behind Alembic head. Master must be on the trunk branch.
func runMasterMigrate(layout liftoff.Layout, oldHead string) migrateResult {
	branch, err := layout.MasterBranch()
	if err != nil {
		return migrateResult{status: migrateSkippedNotTrunk, branch: "?"}
	}
	if branch != layout.MainBranch {
		return migrateResult{status: migrateSkippedNotTrunk, branch: branch}
	}

	newHead, _ := layout.MasterHead()
	newMigs, _ := layout.NewMigrations(oldHead, newHead)

	atHead, err := layout.AlembicAtHead()
	if err != nil {
		return migrateResult{status: migrateFailed, err: err}
	}
	if atHead {
		return migrateResult{status: migrateAtHead}
	}

	fmt.Println()
	title := "master DB behind head — alembic upgrade head"
	if len(newMigs) > 0 {
		title = fmt.Sprintf("%d new migration(s) on master — alembic upgrade head", len(newMigs))
	}
	fmt.Println(tui.StyleTitle.Render(title))
	for _, m := range newMigs {
		fmt.Printf("  %s\n", tui.StyleDim.Render(m))
	}
	if err := layout.AlembicUpgradeHead(func(line string) {
		fmt.Println("  " + tui.StyleDim.Render(line))
	}); err != nil {
		return migrateResult{status: migrateFailed, err: err}
	}
	return migrateResult{status: migrateUpgraded}
}
