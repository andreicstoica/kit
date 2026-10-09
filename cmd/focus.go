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
	focusBackend  string
)

var focusCmd = &cobra.Command{
	Use:               "focus [name]",
	Short:             "Make a worktree the active development environment",
	Long:              "focus opens or reuses the worktree's configured terminal workspace, optionally opens an editor, and connects its viewer unless --no-attach is set. Rex uses the native app; Herdr attaches this terminal or opens Ghostty with --ghostty. --backend overrides the configured runtime.",
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
			Backend:  focusBackend,
		})
	},
}

func init() {
	focusCmd.Flags().StringVarP(&focusEditor, "editor", "e", "", "also open this editor")
	focusCmd.Flags().BoolVar(&focusCursor, "cursor", false, "also open Cursor")
	focusCmd.Flags().BoolVar(&focusGhostty, "ghostty", false, "open a Herdr client in Ghostty")
	focusCmd.Flags().BoolVar(&focusNoAttach, "no-attach", false, "ensure workspace without connecting a viewer")
	focusCmd.Flags().StringVar(&focusLayout, "layout", "", "Kit terminal layout (default, detailed, ai, or a configured layout)")
	focusCmd.Flags().StringVar(&focusBackend, "backend", "", "terminal backend: rex or herdr (default: config)")
	rootCmd.AddCommand(focusCmd)
}
