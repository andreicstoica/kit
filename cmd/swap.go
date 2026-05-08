package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var swapCmd = &cobra.Command{
	Use:     "swap [name] [editor]",
	Aliases: []string{"open"},
	Short:   "Sub into a kit — open its worktree in your IDE",
	Long: "**swap** opens a kit's worktree in your editor.\n\n" +
		"## Examples\n\n" +
		"```\n" +
		"kit swap                       # picker (kit) → picker (editor)\n" +
		"kit swap notebook              # default editor (auto-pick)\n" +
		"kit swap notebook cursor       # force cursor\n" +
		"kit swap notebook zed          # force zed\n" +
		"```\n\n" +
		"## Editor selection\n\n" +
		"With 0 args: pick from installed editors (zed/cursor/code).\n" +
		"With 1 arg (`<name>` only): first match wins among `$KIT_EDITOR`, " +
		"`$VISUAL`, `$EDITOR`, zed, cursor, code.",
	Args: cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()

		var name string
		var editor string
		switch len(args) {
		case 0:
			n, err := tui.PickWorktree(layout, "kit swap — pick a kit")
			if err != nil {
				return err
			}
			if n == "" {
				return nil // user esc'd
			}
			name = n
			e, err := tui.PickEditor(installedEditors())
			if err != nil {
				return err
			}
			if e == "" {
				return nil // user esc'd
			}
			editor = e
		case 1, 2:
			n, err := liftoff.NormalizeAndValidate(args[0])
			if err != nil {
				return err
			}
			name = n
			if len(args) == 2 {
				editor = args[1]
			} else {
				editor = pickEditor()
			}
		}

		path := layout.WorktreePath(name)
		if _, err := os.Stat(path); err != nil {
			legacy := layout.LegacyWorktreePath(name)
			if _, err2 := os.Stat(legacy); err2 == nil {
				path = legacy
			} else {
				return fmt.Errorf("worktree not found: %s", path)
			}
		}

		if editor == "" {
			return fmt.Errorf("no editor found — pass one as 2nd arg or set KIT_EDITOR")
		}
		if _, err := exec.LookPath(editor); err != nil {
			return fmt.Errorf("editor %q not on PATH", editor)
		}

		c := exec.Command(editor, path)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Start(); err != nil {
			return err
		}
		// Best-effort: bump last_used so lineup sorts this kit to the top.
		if st, err := liftoff.LoadState(); err == nil {
			st.TouchLastUsed(name)
			_ = st.Save()
		}
		fmt.Printf("opened %s in %s\n", path, editor)
		return nil
	},
}

// pickEditor picks the first installed editor by env-var order then default
// candidates. Used when the user gives a name but no explicit editor arg.
func pickEditor() string {
	candidates := []string{
		os.Getenv("KIT_EDITOR"),
		os.Getenv("VISUAL"),
		os.Getenv("EDITOR"),
		"zed",
		"cursor",
		"code",
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, err := exec.LookPath(c); err == nil {
			return c
		}
	}
	return ""
}

// installedEditors returns the editor candidate list with install state
// resolved, for the picker. Order is the recommended preference.
func installedEditors() []tui.EditorCandidate {
	defs := []tui.EditorCandidate{
		{Name: "Zed", Binary: "zed", Desc: "open in Zed"},
		{Name: "Cursor", Binary: "cursor", Desc: "open in Cursor"},
		{Name: "VS Code", Binary: "code", Desc: "open in VS Code"},
	}
	// Optional: respect $KIT_EDITOR / $VISUAL / $EDITOR by promoting them.
	if v := os.Getenv("KIT_EDITOR"); v != "" {
		defs = append([]tui.EditorCandidate{
			{Name: v, Binary: v, Desc: "from $KIT_EDITOR"},
		}, defs...)
	}
	for i, e := range defs {
		if _, err := exec.LookPath(e.Binary); err == nil {
			defs[i].Installed = true
		}
	}
	return defs
}

func init() {
	rootCmd.AddCommand(swapCmd)
}
