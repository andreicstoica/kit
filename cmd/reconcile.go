package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var reconcileYes bool

var reconcileCmd = &cobra.Command{
	Use:   "reconcile",
	Short: "Clean config records left by removed worktrees",
	Long:  "Finds Kit records with no matching Git worktree, then removes their saved service state, database, Herdr workspace, gtab file, and port slot. Local branches are kept for safety.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()
		if !layout.MasterIsRepo() {
			return fmt.Errorf("master repo not found at %s", layout.Master)
		}
		return offerOrphanReconcile(layout, reconcileYes)
	},
}

func init() {
	reconcileCmd.Flags().BoolVar(&reconcileYes, "yes", false, "clean all reported orphan records without prompting")
	rootCmd.AddCommand(reconcileCmd)
}

func offerOrphanReconcile(layout liftoff.Layout, yes bool) error {
	candidates, err := layout.FindOrphanedWorktrees()
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		fmt.Println(tui.StyleOK.Render("✓ no orphaned kit records."))
		return nil
	}

	fmt.Println(tui.StyleTitle.Render(fmt.Sprintf("found %d orphaned kit record(s)", len(candidates))))
	for _, candidate := range candidates {
		pending := ""
		if candidate.CleanupPending {
			pending = " " + tui.StyleWarn.Render("(wash pending)")
		}
		fmt.Printf("  %s%s  %s\n", candidate.Name, pending,
			tui.StyleDim.Render("["+strings.Join(candidate.Resources(), ", ")+"]"))
	}
	if !yes {
		accept, err := tui.RunConfirm(tui.ConfirmConfig{
			Title:       "Clean these orphaned records?",
			Affirmative: "Clean",
			Negative:    "Skip",
			Default:     true,
		})
		if err != nil {
			return err
		}
		if !accept {
			return nil
		}
	}

	var errs []error
	for _, candidate := range candidates {
		if err := layout.ReconcileOrphan(candidate, func(line string) {
			fmt.Printf("  %s: %s\n", candidate.Name, tui.StyleDim.Render(line))
		}); err != nil {
			fmt.Println(tui.StyleErr.Render("✗ " + candidate.Name + ": " + err.Error()))
			errs = append(errs, fmt.Errorf("%s: %w", candidate.Name, err))
			continue
		}
		fmt.Println(tui.StyleOK.Render("✓ cleaned " + candidate.Name))
	}
	return errors.Join(errs...)
}
