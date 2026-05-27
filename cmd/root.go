package cmd

import (
	"runtime/debug"

	"github.com/spf13/cobra"
)

// version is injected by the Makefile via -ldflags. "dev" otherwise (e.g.
// `go install`), in which case Version() falls back to the module's VCS tag.
var version = "dev"

func init() {
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if err := MaybeOfferSetup(cmd, args); err != nil {
			return err
		}
		maybeNudgeUpdate(cmd)
		return nil
	}
	rootCmd.RunE = runRootMenu
}

var rootCmd = &cobra.Command{
	Use:   "kit",
	Short: "Manage Liftoff feature worktrees with port allocation + service spin-up",
	Long: "**kit** creates, lists, and runs isolated git-worktree feature environments for Liftoff.\n\n" +
		"## Soccer-themed verbs (classic aliases work)\n\n" +
		"- `design` (`new`) — create a fresh kit\n" +
		"- `lineup` (`ls`) — show kits available\n" +
		"- `play` (`start`) — spin up dev servers\n" +
		"- `pause` (`stop`) — halt services\n" +
		"- `restart` (`bounce`) — stop then start (bounce a hung service)\n" +
		"- `log` (`logs`) — tail service logs\n" +
		"- `wash` (`rm`, `remove`, `delete`) — strip a kit (`--merged` bulk-washes merged/closed)\n" +
		"- `swap` (`open`, `gtab`) — open the worktree in your IDE (`--workspace` for Ghostty)\n" +
		"- `links` (`urls`, `ports`) — print the worktree's URLs\n" +
		"- `diff` — show the worktree's diff vs master (via lumen if installed)\n" +
		"- `doctor` (`physio`) — check your setup\n" +
		"- `setup` — install missing tools, clone master\n" +
		"- `update` — rebuild kit at the latest release\n\n" +
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

// Version returns the build version. Prefers the ldflag-injected value (make
// builds); for `go install` builds it falls back to the module's VCS tag
// embedded in the build info, so `kit --version` stays meaningful.
func Version() string {
	if version != "dev" && version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return version
}
