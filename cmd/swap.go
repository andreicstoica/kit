package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var (
	swapEditorFlag string
	swapWorkspace  bool
	swapDetailed   bool
)

var swapCmd = &cobra.Command{
	Use:     "swap [name]",
	Aliases: []string{"open", "gtab"},
	Short:   "Sub into a kit — open its worktree in your IDE (or --workspace for Ghostty)",
	Long: "**swap** opens a kit's worktree in your editor.\n\n" +
		"## Examples\n\n" +
		"```\n" +
		"kit swap                   # kit picker → editor picker\n" +
		"kit swap notebook          # editor picker\n" +
		"kit swap -e zed            # kit picker → opens in zed\n" +
		"kit swap notebook -e zed   # opens immediately\n" +
		"kit swap -w                # skip editor → Ghostty workspace (2 tabs)\n" +
		"kit swap -w -d notebook    # Ghostty workspace, detailed (5 tabs)\n" +
		"```\n\n" +
		"## Flags\n\n" +
		"`-e` / `--editor` accepts: `zed`, `cursor`, `code`, or any binary on PATH.\n" +
		"Honors `$KIT_EDITOR` if no flag is given and only one editor is installed.\n\n" +
		"`-w` / `--workspace` skips the editor and launches the Ghostty gtab " +
		"workspace directly; `-d` / `--detailed` selects the 5-tab layout. " +
		"(Ghostty is also offered in the editor picker when no flag is given.)\n\n" +
		"On macOS, editors are detected via `.app` bundle in `/Applications` " +
		"OR a CLI binary on PATH. Bundle-only installs are launched via `open -a`.",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()

		name, err := resolveTarget(layout, args, "kit swap — pick a kit")
		if err != nil {
			return err
		}
		if name == "" {
			return nil
		}
		path, err := layout.ResolveWorktreePath(name)
		if err != nil {
			return err
		}

		// One shared entry point — same open behavior as `kit design`'s
		// post-create prompt and anywhere else worktrees are opened.
		_, err = tui.OpenWorktree(tui.OpenRequest{
			Layout:        layout,
			Name:          name,
			Path:          path,
			EditorFlag:    swapEditorFlag,
			WorkspaceOnly: swapWorkspace,
			Detailed:      swapDetailed,
		})
		return err
	},
}

// worktreeFromCwd returns the worktree name if pwd is inside one. Includes
// master (returned as "master"). Returns "" if pwd is unrelated.
func worktreeFromCwd(layout liftoff.Layout) string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	cwd, _ = filepath.Abs(cwd)
	wts, err := layout.ListWorktrees()
	if err != nil {
		return ""
	}
	best := ""
	bestLen := 0
	for _, w := range wts {
		if w.Bare {
			continue
		}
		wp, _ := filepath.Abs(w.Path)
		if cwd == wp || strings.HasPrefix(cwd, wp+string(filepath.Separator)) {
			if len(wp) > bestLen {
				if w.IsMaster(layout) {
					best = "master"
				} else {
					best = w.Name()
				}
				bestLen = len(wp)
			}
		}
	}
	return best
}

func init() {
	swapCmd.Flags().StringVarP(&swapEditorFlag, "editor", "e", "", "editor to open with (zed, cursor, code, or any PATH binary)")
	swapCmd.Flags().BoolVarP(&swapWorkspace, "workspace", "w", false, "skip editor; launch the Ghostty gtab workspace")
	swapCmd.Flags().BoolVarP(&swapDetailed, "detailed", "d", false, "with --workspace: use the 5-tab detailed layout")
	rootCmd.AddCommand(swapCmd)
}
