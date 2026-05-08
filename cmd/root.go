package cmd

import (
	"os"

	"github.com/andreicstoica/kit/internal/tui"
	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
)

var version = "0.1.0"

// kitLog is the shared charm-log instance.
var kitLog = newLogger()

func newLogger() *log.Logger {
	l := log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: false,
		Prefix:          "kit",
		Level:           log.InfoLevel,
	})
	styles := log.DefaultStyles()
	styles.Levels[log.ErrorLevel] = styles.Levels[log.ErrorLevel].Foreground(tui.ColorErr).Bold(true)
	styles.Levels[log.WarnLevel] = styles.Levels[log.WarnLevel].Foreground(tui.ColorWarn).Bold(true)
	styles.Levels[log.InfoLevel] = styles.Levels[log.InfoLevel].Foreground(tui.ColorAccent).Bold(true)
	l.SetStyles(styles)
	return l
}

var rootCmd = &cobra.Command{
	Use:   "kit",
	Short: "Manage Liftoff feature worktrees with port allocation + service spin-up",
	Long: "**kit** creates, lists, and runs isolated git-worktree feature environments for Liftoff.\n\n" +
		"## Soccer-themed verbs (classic aliases work)\n\n" +
		"- `design` (`new`) — create a fresh kit\n" +
		"- `lineup` (`ls`) — show kits on the field\n" +
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

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		kitLog.Error(err.Error())
		os.Exit(1)
	}
}
