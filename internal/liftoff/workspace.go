package liftoff

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// This file holds editor detection and Ghostty workspace launch. Open-target
// taxonomy and the unified picker list live in open_targets.go; Herdr session
// mechanics live in herdr.go. internal/tui/open.go is the single dispatch
// entry for every "open this worktree somewhere" flow.

// EditorCandidate describes one possible editor + its install state. On
// macOS an editor may be installed as a `.app` bundle without a PATH binary,
// so UseOpen records whether to launch via `open -a App` vs the CLI binary.
type EditorCandidate struct {
	Name      string
	Binary    string // CLI binary name (preferred when on PATH unless AppOnly)
	App       string // .app bundle name (e.g. "Zed.app") for `open -a`
	Desc      string
	Installed bool
	UseOpen   bool // true when launch is via `open -a App` not `binary`
	AppOnly   bool // always launch via the .app bundle; ignore any PATH binary
}

// editorDefs is the canonical candidate list, ordered by preference.
func editorDefs() []EditorCandidate {
	return []EditorCandidate{
		{Name: "Zed", Binary: "zed", App: "Zed.app", Desc: "open in Zed"},
		{Name: "Cursor", Binary: "cursor", App: "Cursor.app", Desc: "open in Cursor"},
		{Name: "VS Code", Binary: "code", App: "Visual Studio Code.app", Desc: "open in VS Code"},
	}
}

// ensureZedOffered guarantees Zed is always one of the offered editors, even
// when app-bundle / PATH detection misses it — Zed is the default editor and
// the user expects it in every worktree-open picker. A PATH hit launches via
// the `zed` binary; otherwise we fall back to `open -a Zed.app`. No-op when a
// Zed candidate is already present (so we never double it).
func ensureZedOffered(out []EditorCandidate) []EditorCandidate {
	for _, e := range out {
		if e.Name == "Zed" {
			return out
		}
	}
	zed := EditorCandidate{Name: "Zed", Binary: "zed", App: "Zed.app", Desc: "open in Zed", Installed: true, UseOpen: true}
	if _, err := exec.LookPath("zed"); err == nil {
		zed.UseOpen = false
	}
	// Prepend — Zed is the preferred default, so it leads the list.
	return append([]EditorCandidate{zed}, out...)
}

// EditorNames returns the display names of all known editor candidates, for
// user-facing "looked for ..." messages so they stay in sync with editorDefs.
func EditorNames() []string {
	defs := editorDefs()
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	return names
}

// InstalledEditors returns only candidates that are actually installed.
// Known editors prioritize the .app bundle to avoid squatted PATH binaries
// (e.g. `code` is often Cursor's shim, not VS Code). $KIT_EDITOR is promoted
// to the front and resolved via PATH only.
//
// When the Ghostty.app bundle is present, a synthetic "Ghostty workspace"
// candidate (Binary == WorkspaceSentinel) is appended so a single picker can
// offer both editors and the dev-workspace flow.
func InstalledEditors() []EditorCandidate {
	defs := editorDefs()
	if v := os.Getenv("KIT_EDITOR"); v != "" {
		defs = append([]EditorCandidate{
			{Name: v, Binary: v, Desc: "from $KIT_EDITOR"},
		}, defs...)
	}
	out := make([]EditorCandidate, 0, len(defs))
	for _, e := range defs {
		c := e
		if c.App != "" {
			if appBundleExists(c.App) {
				c.Installed = true
				c.UseOpen = true
				if !c.AppOnly {
					if _, err := exec.LookPath(c.Binary); err == nil {
						c.UseOpen = false
					}
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
	// Zed is always offered, even if detection above missed it.
	out = ensureZedOffered(out)
	if appBundleExists("Ghostty.app") {
		out = append(out, WorkspaceCandidate())
	}
	return out
}

// ResolveEditor returns a candidate for an explicit user-supplied editor
// name. Tries PATH first, then a matching .app bundle alias. Returns nil when
// the named editor isn't found.
func ResolveEditor(name string) *EditorCandidate {
	for _, def := range editorDefs() {
		if def.Binary == name || strings.EqualFold(def.Name, name) {
			c := def
			if !c.AppOnly {
				if _, err := exec.LookPath(c.Binary); err == nil {
					c.Installed = true
					return &c
				}
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
		return &EditorCandidate{Name: name, Binary: name, Installed: true}
	}
	return nil
}

// LaunchEditor opens path in the given editor, via `open -a` for bundle-only
// installs or the CLI binary otherwise.
func LaunchEditor(c EditorCandidate, path string) error {
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

// OpenWorkspace writes the gtab layout, launches the Ghostty workspace, and
// records the worktree as last-used. The single workspace-open path shared by
// `kit swap`, `kit design`, and anything else that opens a dev workspace.
func OpenWorkspace(layout Layout, name, path string, gl GtabLayout) error {
	if _, err := layout.WriteGtabLayout(name, path, gl); err != nil {
		return fmt.Errorf("write gtab: %w", err)
	}
	if err := layout.LaunchGtab(name); err != nil {
		return err
	}
	TouchLastUsedName(name)
	return nil
}

// TouchLastUsedName bumps the worktree's LastUsed timestamp. No-op for master,
// which has no config entry.
func TouchLastUsedName(name string) {
	if name == "master" {
		return
	}
	_ = WithConfigLock(func(c *Config) error {
		c.TouchLastUsed(name)
		return nil
	})
}

// appBundleCache memoizes stat results for the canonical editor bundles so
// InstalledEditors and ResolveEditor don't redo the same syscalls.
var (
	appBundleCache   = map[string]bool{}
	appBundleCacheMu sync.Mutex
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
