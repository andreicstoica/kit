package cmd

import (
	"fmt"

	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

// runRootMenu fires when `kit` is invoked with no subcommand. Designers
// don't have to memorize verbs — pick from a short list of common
// actions and kit dispatches to the matching command.
func runRootMenu(cmd *cobra.Command, args []string) error {
	type item struct {
		verb string
		desc string
	}
	items := []item{
		{"lineup", "see all my kits (--tree for tree view)"},
		{"play", "start a kit's dev servers"},
		{"pause", "stop a kit's dev servers"},
		{"restart", "bounce a kit's services (stop then start)"},
		{"swap", "open a kit in your editor (or Ghostty workspace)"},
		{"log", "tail a kit's logs"},
		{"links", "print a kit's URLs"},
		{"diff", "see what changed vs master"},
		{"design", "create a new kit"},
		{"wash", "remove a kit (worktree + branch + DB + gtab)"},
		{"sync", "pull master + clean up merged branches"},
		{"setup", "install/check required tools"},
		{"doctor", "diagnose without changing anything"},
		{"update", "update kit to the latest release"},
		{"guide", "show the kit guide / daily flow"},
	}
	opts := make([]tui.SelectOption[string], 0, len(items))
	for _, it := range items {
		label := fmt.Sprintf("%-10s — %s", it.verb, it.desc)
		opts = append(opts, tui.SelectOption[string]{Label: label, Value: it.verb})
	}
	picked, err := tui.RunSelect("kit · what do you want to do?", "pick an action, or Ctrl-C to exit", opts, items[0].verb)
	if err != nil {
		return err
	}
	if picked == "" {
		return nil
	}
	// Dispatch by walking the cobra tree.
	for _, sub := range rootCmd.Commands() {
		if sub.Name() == picked {
			return sub.RunE(sub, nil)
		}
	}
	return fmt.Errorf("unknown action: %s", picked)
}
