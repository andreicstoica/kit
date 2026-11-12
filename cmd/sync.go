package cmd

import (
	"errors"
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
		"A failed `gt sync` still fetches and fast-forwards trunk so the merged scan runs; the Graphite error is reported at the end.\n\n" +
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
	go func() { _, _ = liftoff.SweepOldRunDirs(logRetention) }()
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
	gtErr := gt.Run()
	var retErr error
	if gtErr != nil {
		// A merge outside Graphite (for example through Linear) can leave gt's
		// stack state unhappy. Keep the scan useful by refreshing trunk
		// directly, and report the Graphite failure at the end.
		retErr = fmt.Errorf("gt sync failed: %w", gtErr)
		refreshTrunkAfterFailedSync(layout)
	}

	migrate := migrateResult{status: migrateSkippedNoMigrate}
	if !syncSkipMigrate {
		migrate = runMasterMigrate(layout, oldHead)
	}

	if migrate.err != nil {
		retErr = errors.Join(retErr, fmt.Errorf("master DB migration failed: %w", migrate.err))
	}
	if syncSkipTear {
		printMigrateSummary(layout, migrate)
		return retErr
	}

	retErr = errors.Join(retErr, offerMergedWash(layout))
	printMigrateSummary(layout, migrate)
	return retErr
}

// refreshTrunkAfterFailedSync keeps the merged scan useful when Graphite is
// stuck: fetch the real trunk and fast-forward it when safe.
func refreshTrunkAfterFailedSync(layout liftoff.Layout) {
	fmt.Println(tui.StyleWarn.Render("! gt sync failed — continuing with a plain trunk refresh"))
	if err := layout.FetchMain(nil); err != nil {
		fmt.Println(tui.StyleWarn.Render("! trunk fetch failed: " + err.Error()))
		return
	}
	ran, err := layout.FastForwardTrunk(nil)
	if err != nil {
		fmt.Println(tui.StyleWarn.Render("! trunk fast-forward failed: " + err.Error()))
	} else if ran {
		fmt.Println(tui.StyleDim.Render("  fast-forwarded " + layout.MainBranch + " to origin/" + layout.MainBranch))
	}
}

// offerMergedWash scans for old workspaces and offers the bulk wash. Orphan
// records are offered afterward, except when the user declines the wash.
func offerMergedWash(layout liftoff.Layout) error {
	scan, err := layout.ScanMergedWorktrees()
	if err != nil {
		return err
	}
	for _, warning := range scan.Warnings {
		fmt.Println(tui.StyleWarn.Render("! " + warning))
	}
	if len(scan.Candidates) == 0 {
		fmt.Println(tui.StyleOK.Render("✓ no merged worktrees to wash."))
		return offerOrphanReconcile(layout, false)
	}

	fmt.Println()
	fmt.Println(tui.StyleTitle.Render(fmt.Sprintf("found %d merged/closed worktree(s)", len(scan.Candidates))))
	for _, c := range scan.Candidates {
		fmt.Printf("  %s  %s\n", c.Name, tui.StyleDim.Render("("+c.Reason+")"))
	}
	fmt.Println()

	accept, err := tui.RunConfirm(tui.ConfirmConfig{
		Title:    "Run `kit wash --merged` to wash them?",
		Negative: "Skip",
		Default:  true,
	})
	if err != nil || !accept {
		return err
	}
	return errors.Join(tui.RunMergedWashTUI(layout), offerOrphanReconcile(layout, false))
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
