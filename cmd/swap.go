package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var swapEditorFlag string

var swapCmd = &cobra.Command{
	Use:     "swap [name]",
	Aliases: []string{"open"},
	Short:   "Sub into a kit — open its worktree in your IDE",
	Long: "**swap** opens a kit's worktree in your editor.\n\n" +
		"## Examples\n\n" +
		"```\n" +
		"kit swap                   # kit picker → editor picker\n" +
		"kit swap notebook          # editor picker\n" +
		"kit swap -e zed            # kit picker → opens in zed\n" +
		"kit swap notebook -e zed   # opens immediately\n" +
		"```\n\n" +
		"## Editor flag\n\n" +
		"`-e` / `--editor` accepts: `zed`, `cursor`, `code`, or any binary on PATH.\n" +
		"Honors `$KIT_EDITOR` if no flag is given and only one editor is installed.\n\n" +
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

		// Resolve editor.
		var chosen *tui.EditorCandidate
		if swapEditorFlag != "" {
			c := resolveEditor(swapEditorFlag)
			if c == nil {
				return fmt.Errorf("editor %q not on PATH or in /Applications", swapEditorFlag)
			}
			chosen = c
		} else {
			eds := installedEditors()
			// Skip the picker when there's only one real editor (Ghostty is a
			// secondary target, not an editor). One installed editor + Ghostty
			// = still pick, since two distinct intents.
			if soleEditor := loneEditor(eds); soleEditor != nil {
				chosen = soleEditor
			} else {
				c, err := tui.PickEditor(eds)
				if err != nil {
					return err
				}
				if c == nil {
					return nil
				}
				chosen = c
			}
		}

		path, err := layout.ResolveWorktreePath(name)
		if err != nil {
			return err
		}

		if chosen.Binary == warmupBinarySentinel {
			gl, err := tui.PickGtabLayout(false)
			if err != nil {
				return err
			}
			if _, err := layout.WriteGtabLayout(name, path, gl); err != nil {
				return fmt.Errorf("write gtab: %w", err)
			}
			if err := layout.LaunchGtab(name); err != nil {
				return err
			}
		} else {
			if err := launchEditor(*chosen, path); err != nil {
				return err
			}
		}
		// Skip state touch for master — it has no entry in state.toml.
		if name != "master" {
			if st, err := liftoff.LoadState(); err == nil {
				st.TouchLastUsed(name)
				_ = st.Save()
			}
		}
		fmt.Printf("opened %s in %s\n", path, chosen.Name)
		return nil
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

// editorDefs is the canonical candidate list, ordered by preference.
func editorDefs() []tui.EditorCandidate {
	return []tui.EditorCandidate{
		{Name: "Zed", Binary: "zed", App: "Zed.app", Desc: "open in Zed"},
		{Name: "Cursor", Binary: "cursor", App: "Cursor.app", Desc: "open in Cursor"},
		{Name: "VS Code", Binary: "code", App: "Visual Studio Code.app", Desc: "open in VS Code"},
	}
}

// installedEditors returns only candidates that are actually installed.
// Known editors prioritize the .app bundle to avoid squatted PATH binaries
// (e.g. `code` is often Cursor's shim, not VS Code). $KIT_EDITOR is promoted
// to the front and resolved via PATH only.
//
// Always appends a "Ghostty (gtab workspace)" candidate when Ghostty.app
// is present, so swap's picker can also launch the warmup flow.
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
		if c.App != "" {
			if appBundleExists(c.App) {
				c.Installed = true
				c.UseOpen = true
				if _, err := exec.LookPath(c.Binary); err == nil {
					c.UseOpen = false
				}
				out = append(out, c)
			}
			continue
		}
		if _, err := exec.LookPath(c.Binary); err == nil {
			c.Installed = true
			out = append(out, c)
		}
	}
	if appBundleExists("Ghostty.app") {
		out = append(out, tui.EditorCandidate{
			Name:      "Ghostty (pick layout next)",
			Binary:    warmupBinarySentinel,
			App:       "Ghostty.app",
			Desc:      "dev workspace — simple (2 tabs) or detailed (5 tabs)",
			Installed: true,
		})
	}
	return out
}

// warmupBinarySentinel marks the synthetic Ghostty-warmup candidate so swap's
// RunE can route to LaunchGtab instead of launchEditor.
const warmupBinarySentinel = "__warmup__"

// loneEditor returns the single installed editor candidate when no
// editor picker is necessary. Returns nil when there are zero installed
// editors, two or more editors, or any number plus the Ghostty target
// (Ghostty represents a distinct intent so the picker still appears).
func loneEditor(eds []tui.EditorCandidate) *tui.EditorCandidate {
	var editors []tui.EditorCandidate
	hasGhostty := false
	for _, e := range eds {
		if e.Binary == warmupBinarySentinel {
			hasGhostty = true
			continue
		}
		editors = append(editors, e)
	}
	if len(editors) == 1 && !hasGhostty {
		c := editors[0]
		return &c
	}
	return nil
}

// resolveEditor returns a candidate for an explicit user-supplied editor name.
// Tries PATH first, then matching .app bundle alias.
func resolveEditor(name string) *tui.EditorCandidate {
	for _, def := range editorDefs() {
		if def.Binary == name || strings.EqualFold(def.Name, name) {
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
	if _, err := exec.LookPath(name); err == nil {
		return &tui.EditorCandidate{Name: name, Binary: name, Installed: true}
	}
	return nil
}

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

// appBundleCache memoizes stat results for the canonical editor bundles so
// installedEditors() and resolveEditor() don't redo the same syscalls.
var (
	appBundleCache    = map[string]bool{}
	appBundleCacheMu  sync.Mutex
)

func appBundleExists(app string) bool {
	appBundleCacheMu.Lock()
	defer appBundleCacheMu.Unlock()
	if v, ok := appBundleCache[app]; ok {
		return v
	}
	for _, root := range []string{"/Applications", filepath.Join(os.Getenv("HOME"), "Applications")} {
		if _, err := os.Stat(filepath.Join(root, app)); err == nil {
			appBundleCache[app] = true
			return true
		}
	}
	appBundleCache[app] = false
	return false
}

func init() {
	swapCmd.Flags().StringVarP(&swapEditorFlag, "editor", "e", "", "editor to open with (zed, cursor, code, or any PATH binary)")
	rootCmd.AddCommand(swapCmd)
}
