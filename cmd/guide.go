package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

const guideBody = `# kit · daily flow

A short tour of the commands you'll use most. All verbs accept a name,
or pick from a numbered list, or auto-detect the worktree you're in.

## First time on this machine

` + "`" + `kit setup` + "`" + ` installs missing tools (brew, gt, postgres, node, yarn…),
authenticates ` + "`" + `gh` + "`" + `, clones the Liftoff master repo, and adopts any
existing worktrees. Idempotent — safe to re-run any time.

` + "`" + `kit doctor` + "`" + ` is the read-only version: lists what's installed, what's
missing, and what you'd need to fix. Doesn't change anything.

## See what you've got

` + "`" + `kit lineup` + "`" + ` is the at-a-glance table — name, slot, running services,
branch, status. Adds ` + "`" + `--tree` + "`" + ` for a hierarchical view with each
worktree's gt stack inlined.

## Make a new kit

` + "`" + `kit design feat-name` + "`" + ` walks a wizard: branch off master, copy env
files, optional DB clone + backend deps + node_modules symlink +
graphite track, write a Ghostty workspace, allocate a port slot.

## Run / stop services

` + "`" + `kit play` + "`" + ` (alias ` + "`" + `start` + "`" + `) brings up the worktree's dev servers
on its slot's port band. ` + "`" + `kit pause` + "`" + ` (alias ` + "`" + `stop` + "`" + `) takes them
down. Toggle screen shows which are running and which kit will start.

## Open things

` + "`" + `kit swap` + "`" + ` (alias ` + "`" + `open` + "`" + `) opens the worktree in your editor.
Picker includes Ghostty if installed — selecting it launches the gtab
4-tab workspace (frontend split, backend split, celery, all auto-tailing
their service logs).

` + "`" + `kit warmup` + "`" + ` opens just the Ghostty workspace, no editor.

` + "`" + `kit links` + "`" + ` (aliases ` + "`" + `urls` + "`" + `, ` + "`" + `ports` + "`" + `) prints the worktree's
URLs — paste into Slack/Linear without doing port math.

## Look at logs

` + "`" + `kit log` + "`" + ` (alias ` + "`" + `logs` + "`" + `) opens a multi-tail viewer over every
service log for the worktree, color-coded by service. ` + "`" + `f` + "`" + ` toggles
follow, ` + "`" + `/` + "`" + ` filters by substring, ` + "`" + `q` + "`" + ` quits.

## See what changed

` + "`" + `kit diff` + "`" + ` runs an interactive diff vs master. Uses ` + "`" + `lumen` + "`" + ` for
a side-by-side viewer if installed, falls back to ` + "`" + `git diff` + "`" + `.

## Clean up

` + "`" + `kit sync` + "`" + ` runs ` + "`" + `gt sync` + "`" + ` in master (pulls trunk, restacks,
prunes merged local branches), then offers to wash any worktree whose
branch is merged or whose PR is closed.

` + "`" + `kit tear` + "`" + ` (alias ` + "`" + `prune` + "`" + `) is the standalone version of that
prune step — multi-select picker over merged/closed worktrees.

` + "`" + `kit wash` + "`" + ` (aliases ` + "`" + `rm` + "`" + `, ` + "`" + `remove` + "`" + `, ` + "`" + `delete` + "`" + `) strips a single
kit — worktree dir, branch, optional DB, gtab, slot.

## Troubleshooting

` + "`" + `kit adopt` + "`" + ` registers a worktree that already exists on disk but
isn't in kit's config yet. Setup runs this automatically for legacy
worktrees.

Run ` + "`" + `kit doctor` + "`" + ` (alias ` + "`" + `physio` + "`" + `) when something seems off — it
tells you what's missing without changing your machine.
`

var guideCmd = &cobra.Command{
	Use:   "guide",
	Short: "Show the kit daily-flow guide",
	Long:  guideBody,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Long is glamour-rendered at startup by RenderMarkdownLongs,
		// so simply printing it here gives the styled output.
		fmt.Println(cmd.Long)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(guideCmd)
}
