package cmd

import (
	"fmt"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var (
	openLayout string
	openEditor string
	openHerdr  bool
)

var openCmd = &cobra.Command{
	Use:   "open [name]",
	Short: "Open a worktree in an editor or its persistent Herdr space",
	Long: "open picks a worktree, then asks how to open it: any installed editor (Zed, Cursor, …) rooted at " +
		"that worktree's checkout, or the worktree's persistent Herdr space. Choosing Herdr creates the space " +
		"and Kit-owned tabs once — offering the Simple (2 tabs) or Detailed (5 tabs) layout on first open — then " +
		"attaches the shared session. --editor and --herdr skip the picker.",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		if openEditor != "" && openHerdr {
			return fmt.Errorf("choose one of --editor or --herdr")
		}
		layout := liftoff.DefaultLayout()
		name, err := resolveTarget(layout, args, "kit open — pick a worktree")
		if err != nil || name == "" {
			return err
		}
		path, err := layout.ResolveWorktreePath(name)
		if err != nil {
			return err
		}

		if openEditor != "" {
			editor := liftoff.ResolveEditor(openEditor)
			if editor == nil {
				return fmt.Errorf("editor %q not on PATH or in /Applications", openEditor)
			}
			return launchEditorAt(cmd, *editor, name, path)
		}
		if openHerdr {
			return openHerdrSpace(cmd, name, path)
		}

		// Interactive: installed editors plus the Herdr space, so opening a
		// worktree just to read code no longer implies starting a Herdr
		// session. Herdr goes last because the editors are the common case.
		choice, err := tui.PickEditor(append(liftoff.InstalledEditors(), liftoff.HerdrCandidate()))
		if err != nil || choice == nil {
			return err
		}
		if choice.Binary == liftoff.HerdrSentinel {
			return openHerdrSpace(cmd, name, path)
		}
		return launchEditorAt(cmd, *choice, name, path)
	},
}

// launchEditorAt opens the worktree's checkout as the editor's root, so the
// editor — and any agent running inside it — starts on the right project
// rather than on whatever directory the shell happened to be in.
func launchEditorAt(cmd *cobra.Command, editor liftoff.EditorCandidate, name, path string) error {
	if err := liftoff.LaunchEditor(editor, path); err != nil {
		return err
	}
	liftoff.TouchLastUsedName(name)
	fmt.Fprintf(cmd.OutOrStdout(), "opened %s in %s\n", path, editor.Name)
	return nil
}

func openHerdrSpace(cmd *cobra.Command, name, path string) error {
	chosenLayout, err := resolveHerdrLayout(name, path, openLayout)
	if err != nil {
		return err
	}
	workspace, err := liftoff.OpenHerdr(name, path, chosenLayout)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "opened %s in Herdr (%s)\n", name, workspace.WorkspaceID)
	return liftoff.AttachHerdr()
}

func init() {
	openCmd.Flags().StringVar(&openLayout, "layout", "", "Kit Herdr layout (default, detailed, ai, or a configured layout)")
	openCmd.Flags().StringVarP(&openEditor, "editor", "e", "", "open this editor directly (zed, cursor, code, or any PATH binary)")
	openCmd.Flags().BoolVar(&openHerdr, "herdr", false, "go straight to the persistent Herdr space")
	rootCmd.AddCommand(openCmd)
}
