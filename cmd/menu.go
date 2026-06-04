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
		{"lineup", "see all workspaces"},
		{"play", "start a workspace"},
		{"pause", "stop a workspace"},
		{"restart", "stop and start a workspace"},
		{"swap", "open a workspace in your editor or Ghostty"},
		{"log", "watch app logs"},
		{"links", "show a workspace's URLs"},
		{"diff", "see what changed vs master"},
		{"design", "create a new workspace"},
		{"wash", "delete an old workspace"},
		{"sync", "update master and clean up merged work"},
		{"setup", "install/check required tools"},
		{"doctor", "diagnose without changing anything"},
		{"update", "update kit to the latest release"},
	}
	opts := make([]tui.SelectOption[string], 0, len(items))
	for _, it := range items {
		label := fmt.Sprintf("%-10s — %s", it.verb, it.desc)
		opts = append(opts, tui.SelectOption[string]{Label: label, Value: it.verb})
	}
	picked, err := tui.RunSelect("kit · what do you want to do?", "Choose an action. Press Ctrl-C to exit.", opts, items[0].verb)
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
