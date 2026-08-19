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
			HerdrLayout:  openLayout,
			HerdrConnect: tui.HerdrConnectAttach,
		})
		return err
	},
}

func init() {
	openCmd.Flags().StringVar(&openLayout, "layout", "", "Kit Herdr layout (default, detailed, ai, or a configured layout)")
	openCmd.Flags().StringVarP(&openEditor, "editor", "e", "", "open this editor directly (zed, cursor, code, or any PATH binary)")
	openCmd.Flags().BoolVar(&openHerdr, "herdr", false, "go straight to the persistent Herdr space")
	rootCmd.AddCommand(openCmd)
}
