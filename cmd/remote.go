package cmd

import (
	"fmt"
	"strings"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/charmbracelet/bubbles/list"
	"github.com/spf13/cobra"
)

type remoteWorktreeItem struct {
	name string
	desc string
}

func (i remoteWorktreeItem) Title() string       { return i.name }
func (i remoteWorktreeItem) Description() string { return i.desc }
func (i remoteWorktreeItem) FilterValue() string { return i.name }

var remoteCmd = &cobra.Command{
	Use:               "remote [name]",
	Short:             "Pick a running worktree and attach to its Herdr space",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()
		if err := liftoff.EnsureHerdrServer(); err != nil {
			return err
		}
		state, err := liftoff.ReadHerdrState()
		if err != nil {
			return err
		}

		name := ""
		if len(args) == 1 {
			name, err = resolveArgOrCwd(layout, args)
		} else {
			name, err = pickRemoteWorktree(layout, state)
		}
		if err != nil || name == "" {
			return err
		}
		path, err := layout.ResolveWorktreePath(name)
		if err != nil {
			return err
		}
		return tui.OpenHerdrWorktree(name, path, "", tui.HerdrConnectAttach)
	},
}

func pickRemoteWorktree(layout liftoff.Layout, state liftoff.HerdrState) (string, error) {
	wts, err := layout.ListWorktrees()
	if err != nil {
		return "", err
	}
	cfg, _ := liftoff.LoadConfig()
	items := make([]list.Item, 0, len(wts))
	for _, wt := range wts {
		if wt.Bare {
			continue
		}
		name := wt.Name()
		if wt.IsMaster(layout) {
			name = "master"
		}
		var savedID string
		if cfg != nil {
			savedID = cfg.Worktrees[name].HerdrID
		}
		space := liftoff.FindHerdrWorkspace(state, savedID, name, wt.Path)
		details := []string{"idle"}
		if space != nil {
			details = []string{fmt.Sprintf("%d tabs · %d panes", space.TabCount, space.PaneCount)}
			if agents := state.WorkspaceAgentSummary(space.WorkspaceID); agents != "idle" {
				details = append(details, agents)
			}
		}
		if cfg != nil {
			if meta, ok := cfg.Worktrees[name]; ok && meta.Slot > 0 {
				running, total := liftoff.RunningCount(name, liftoff.PortsForSlot(meta.Slot))
				if running > 0 {
					details = append(details, fmt.Sprintf("services %d/%d", running, total))
				}
			}
		}
		items = append(items, remoteWorktreeItem{name: name, desc: strings.Join(details, " · ")})
	}
	if len(items) == 0 {
		return "", fmt.Errorf("no worktrees found — run `kit design` first")
	}
	chosen, ok, err := tui.RunListPicker(tui.ListPickerConfig{
		Title:  "kit remote — pick a Herdr space",
		Items:  items,
		Filter: true,
	})
	if err != nil || !ok {
		return "", err
	}
	return chosen.(remoteWorktreeItem).name, nil
}

func init() { rootCmd.AddCommand(remoteCmd) }
