package cmd

import (
	"fmt"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/spf13/cobra"
)

var warmupDetailed bool

var warmupCmd = &cobra.Command{
	Use:     "warmup [name]",
	Aliases: []string{"gtab"},
	Short:   "Pre-match warmup: launch the gtab ghostty workspace for a kit",
	Long: "**warmup** opens the kit's ghostty workspace.\n\n" +
		"Default layout is 2 tabs (worktree root + combined `tail -F` over " +
		"every service log). `--detailed` switches to 5 tabs: shell, " +
		"frontend split (app + admin), backend split (api + admin_be), " +
		"celery, combined logs.\n\n" +
		"With no arg, uses the worktree you're in (or the numbered picker if " +
		"cwd is unrelated or master).\n\n" +
		"The AppleScript is regenerated each run so it stays in sync with " +
		"the current layout — safe to remove `~/.config/gtab/<name>.applescript` " +
		"any time.",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()
		name, err := resolveTarget(layout, args, "kit warmup — pick a kit")
		if err != nil {
			return err
		}
		if name == "" {
			return nil
		}
		path, err := layout.ResolveWorktreePath(name)
		if err != nil {
			return err
		}
		gl := liftoff.GtabSimple
		if warmupDetailed {
			gl = liftoff.GtabDetailed
		}
		if _, err := layout.WriteGtabLayout(name, path, gl); err != nil {
			return fmt.Errorf("write gtab: %w", err)
		}
		return layout.LaunchGtab(name)
	},
}

func init() {
	warmupCmd.Flags().BoolVarP(&warmupDetailed, "detailed", "d", false,
		"use the 5-tab detailed layout (per-service tail panes + combined logs)")
	rootCmd.AddCommand(warmupCmd)
}
