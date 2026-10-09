package cmd

import (
	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var (
	openLayout string
	openEditor string
	openHerdr  bool
	openRex    bool
)

var openCmd = &cobra.Command{
	Use:   "open [name]",
	Short: "Open a worktree in an editor, Rex, or Herdr",
	Long: "open picks a worktree, then asks where to open it: an installed editor, a native Rex workspace, or a persistent Herdr space. " +
		"Terminal workspaces reuse their stable IDs and add missing Kit layout tabs without removing your custom tabs. " +
		"--editor, --rex, and --herdr skip the destination picker. Explicit destinations override the configured backend.",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()
		name, err := resolveTarget(layout, args, "kit open — pick a worktree")
		if err != nil || name == "" {
			return err
		}
		path, err := layout.ResolveWorktreePath(name)
		if err != nil {
			return err
		}

		_, err = tui.OpenWorktree(tui.OpenRequest{
			Layout:       layout,
			Name:         name,
			Path:         path,
			EditorFlag:   openEditor,
			Herdr:        openHerdr,
			Rex:          openRex,
			HerdrLayout:  openLayout,
			HerdrConnect: tui.HerdrConnectAttach,
		})
		return err
	},
}

func init() {
	openCmd.Flags().StringVar(&openLayout, "layout", "", "Kit terminal layout (default, detailed, ai, or a configured layout)")
	openCmd.Flags().StringVarP(&openEditor, "editor", "e", "", "open this editor directly (zed, cursor, code, or any PATH binary)")
	openCmd.Flags().BoolVar(&openHerdr, "herdr", false, "go straight to the persistent Herdr space")
	openCmd.Flags().BoolVar(&openRex, "rex", false, "go straight to the persistent Rex workspace")
	rootCmd.AddCommand(openCmd)
}
