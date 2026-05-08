package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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
		"## Editor detection\n\n" +
		"Editors are detected via PATH binary OR `.app` bundle in `/Applications`. " +
		"Zed, Cursor, and VS Code all install as `.app` bundles on macOS — kit " +
		"opens them via `open -a <App>` when the CLI shim is missing.",
	Args: cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()

		var name string
		var chosen *tui.EditorCandidate
		switch len(args) {
		case 0:
			n, err := tui.PickWorktree(layout, "kit swap — pick a kit")
			if err != nil {
				return err
			}
			if n == "" {
				return nil
			}
			name = n
			c, err := tui.PickEditor(installedEditors())
			if err != nil {
				return err
			}
			if c == nil {
				return nil
			}
			chosen = c
		case 1, 2:
			n, err := liftoff.NormalizeAndValidate(args[0])
			if err != nil {
				return err
			}
			name = n
			if len(args) == 2 {
				c := resolveEditor(args[1])
				if c == nil {
					return fmt.Errorf("editor %q not on PATH or in /Applications", args[1])
				}
				chosen = c
			} else {
				chosen = autoPickEditor()
			}
		}

		if chosen == nil {
			return fmt.Errorf("no editor found — install Zed/Cursor/VS Code or pass one as 2nd arg")
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

		if err := launchEditor(*chosen, path); err != nil {
			return err
		}

		if st, err := liftoff.LoadState(); err == nil {
			st.TouchLastUsed(name)
			_ = st.Save()
		}
		fmt.Printf("opened %s in %s\n", path, chosen.Name)
		return nil
	},
}

// editorDefs is the canonical candidate list, ordered by preference.
func editorDefs() []tui.EditorCandidate {
	return []tui.EditorCandidate{
		{Name: "Zed", Binary: "zed", App: "Zed.app", Desc: "open in Zed"},
		{Name: "Cursor", Binary: "cursor", App: "Cursor.app", Desc: "open in Cursor"},
		{Name: "VS Code", Binary: "code", App: "Visual Studio Code.app", Desc: "open in VS Code"},
	}
}

// installedEditors returns only candidates that are actually installed,
// flagging UseOpen=true when the binary is missing but the .app bundle exists.
// Honors $KIT_EDITOR by promoting it to the front.
func installedEditors() []tui.EditorCandidate {
	defs := editorDefs()
	if v := os.Getenv("KIT_EDITOR"); v != "" {
		defs = append([]tui.EditorCandidate{
			{Name: v, Binary: v, Desc: "from $KIT_EDITOR"},
		}, defs...)
	}
	out := make([]tui.EditorCandidate, 0, len(defs))
	for _, e := range defs {
		c := e
		// Known editors (with an App field): require the .app bundle. CLI
		// binaries like `code` are commonly squatted by other tools (e.g.
		// Cursor installs `code`), so PATH alone isn't reliable proof.
		if c.App != "" {
			if appBundleExists(c.App) {
				c.Installed = true
				c.UseOpen = true
				if _, err := exec.LookPath(c.Binary); err == nil {
					// Both available: prefer the binary (faster, no Finder).
					c.UseOpen = false
				}
				out = append(out, c)
			}
			continue
		}
		// Custom editor (no App field — e.g. came in via $KIT_EDITOR).
		if _, err := exec.LookPath(c.Binary); err == nil {
			c.Installed = true
			out = append(out, c)
		}
	}
	return out
}

// autoPickEditor returns the first installed editor honoring env vars first.
func autoPickEditor() *tui.EditorCandidate {
	for _, env := range []string{"KIT_EDITOR", "VISUAL", "EDITOR"} {
		if v := os.Getenv(env); v != "" {
			if c := resolveEditor(v); c != nil {
				return c
			}
		}
	}
	for _, c := range installedEditors() {
		c := c
		return &c
	}
	return nil
}

// resolveEditor returns a candidate for an explicit user-supplied editor name.
// Tries PATH first, then matching app bundle alias.
func resolveEditor(name string) *tui.EditorCandidate {
	// Match a known def first so we get the proper App fallback.
	for _, def := range editorDefs() {
		if def.Binary == name || def.Name == name {
			c := def
			if _, err := exec.LookPath(c.Binary); err == nil {
				c.Installed = true
				return &c
			}
			if c.App != "" && appBundleExists(c.App) {
				c.Installed = true
				c.UseOpen = true
				return &c
			}
			return nil
		}
	}
	// Unknown editor name: fall back to PATH-only resolution.
	if _, err := exec.LookPath(name); err == nil {
		return &tui.EditorCandidate{Name: name, Binary: name, Installed: true}
	}
	return nil
}

// launchEditor execs the editor against the worktree path. Uses `open -a App`
// when only the bundle is present.
func launchEditor(c tui.EditorCandidate, path string) error {
	var cmd *exec.Cmd
	if c.UseOpen {
		cmd = exec.Command("open", "-a", c.App, path)
	} else {
		cmd = exec.Command(c.Binary, path)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}

// appBundleExists checks /Applications and ~/Applications for the bundle.
func appBundleExists(app string) bool {
	for _, root := range []string{"/Applications", filepath.Join(os.Getenv("HOME"), "Applications")} {
		if _, err := os.Stat(filepath.Join(root, app)); err == nil {
			return true
		}
	}
	return false
}

func init() {
	rootCmd.AddCommand(swapCmd)
}
