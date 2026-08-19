package cmd

import (
	"fmt"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var (
	focusEditor   string
	focusCursor   bool
	focusGhostty  bool
	focusNoAttach bool
	focusLayout   string
)

var focusCmd = &cobra.Command{
	Use:               "focus [name]",
	Short:             "Make a worktree the active development environment",
	Long:              "focus opens or reuses the worktree's Herdr space, optionally opens an editor or Ghostty client, and attaches the current terminal unless --no-attach is set. First-time spaces use the Simple (2 tabs) or Detailed (5 tabs) picker.",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		if focusCursor && focusEditor != "" {
			return fmt.Errorf("choose one of --cursor or --editor")
		}
		layout := liftoff.DefaultLayout()
		name, err := resolveTarget(layout, args, "kit focus — pick a worktree")
		if err != nil || name == "" {
			return err
		}
		path, err := layout.ResolveWorktreePath(name)
		if err != nil {
			return err
		}

		editorName := focusEditor
		if focusCursor {
			editorName = "cursor"
		}
		return tui.FocusHerdr(tui.FocusHerdrRequest{
			Name:     name,
			Path:     path,
			Layout:   focusLayout,
			Editor:   editorName,
			Ghostty:  focusGhostty,
			NoAttach: focusNoAttach,
		})
	},
}

func init() {
	focusCmd.Flags().StringVarP(&focusEditor, "editor", "e", "", "also open this editor")
	focusCmd.Flags().BoolVar(&focusCursor, "cursor", false, "also open Cursor")
	focusCmd.Flags().BoolVar(&focusGhostty, "ghostty", false, "open a Herdr client in Ghostty")
	focusCmd.Flags().BoolVar(&focusNoAttach, "no-attach", false, "focus/setup only; do not attach this terminal")
	focusCmd.Flags().StringVar(&focusLayout, "layout", "", "Kit Herdr layout (default, detailed, ai, or a configured layout)")
	rootCmd.AddCommand(focusCmd)
}
