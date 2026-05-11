package cmd

import (
	"github.com/spf13/cobra"
)

var version = "0.1.0"

var rootCmd = &cobra.Command{
	Use:   "kit",
	Short: "Manage Liftoff feature worktrees with port allocation + service spin-up",
	Long: "**kit** creates, lists, and runs isolated git-worktree feature environments for Liftoff.\n\n" +
		"## Soccer-themed verbs (classic aliases work)\n\n" +
		"- `design` (`new`) — create a fresh kit\n" +
		"- `lineup` (`ls`) — show kits available\n" +
		"- `play` — spin up dev servers (frontend + backend + celery)\n" +
		"- `pause` — halt services\n" +
		"- `log` — tail service logs\n" +
		"- `wash` (`rm`) — strip a kit and clean up\n" +
		"- `tear` (`prune`) — bulk-wash merged/closed branches\n" +
		"- `warmup` (`gtab`) — launch the ghostty workspace\n" +
		"- `swap` (`open`) — open the worktree in your IDE\n\n" +
		"## What makes it useful\n\n" +
		"Each worktree gets a unique 5-port slot at `design` time. `kit play feat-a` " +
		"and `kit play feat-b` run side-by-side without port conflicts. Frontend env vars " +
		"are injected at runtime so worktree env files stay textually identical to master.",
	Version:       version,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Root exposes the cobra root for main.go to hand to fang.Execute.
func Root() *cobra.Command { return rootCmd }

// Version returns the ldflags-injected version string for fang.WithVersion.
func Version() string { return version }
