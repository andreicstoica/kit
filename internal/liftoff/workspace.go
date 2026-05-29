package liftoff

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// This file is the single source of truth for "open a worktree" — both the
// editor candidates kit knows about and the launch mechanics. The UI layer
// (internal/tui) presents the picker; the command layer (cmd) wires flags;
// but the domain knowledge of *what* the editors are and *how* to launch
// them lives here so every command opens a worktree identically.

// WorkspaceSentinel is the synthetic EditorCandidate.Binary value marking the
// "Ghostty dev workspace" target. Callers route it to OpenWorkspace instead
// of LaunchEditor.
const WorkspaceSentinel = "__workspace__"

// SkipSentinel marks a "don't open anything" candidate, offered by flows
// (like post-design) that want a no-op escape hatch in the same picker.
const SkipSentinel = "__skip__"

// EditorCandidate describes one possible editor + its install state. On
// macOS an editor may be installed as a `.app` bundle without a PATH binary,
// so UseOpen records whether to launch via `open -a App` vs the CLI binary.
type EditorCandidate struct {
	Name      string
	Binary    string // CLI binary name (preferred when on PATH)
	App       string // .app bundle name (e.g. "Zed.app") for `open -a`
	Desc      string
	Installed bool
	UseOpen   bool // true when launch is via `open -a App` not `binary`
}

// editorDefs is the canonical candidate list, ordered by preference.
func editorDefs() []EditorCandidate {
	return []EditorCandidate{
		{Name: "Zed", Binary: "zed", App: "Zed.app", Desc: "open in Zed"},
		{Name: "Cursor", Binary: "cursor", App: "Cursor.app", Desc: "open in Cursor"},
		{Name: "VS Code", Binary: "code", App: "Visual Studio Code.app", Desc: "open in VS Code"},
	}
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
		out = append(out, EditorCandidate{
			Name:      "Ghostty (pick layout next)",
			Binary:    WorkspaceSentinel,
			App:       "Ghostty.app",
			Desc:      "dev workspace — simple (2 tabs) or detailed (5 tabs)",
			Installed: true,
		})
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
		return &EditorCandidate{Name: name, Binary: name, Installed: true}
	}
	return nil
}

// LoneEditor returns the single installed editor when no picker is needed.
// Returns nil when there are zero editors, two or more editors, or any number
// plus the Ghostty target (Ghostty is a distinct intent, so the picker still
// appears). The WorkspaceSentinel and SkipSentinel candidates are ignored.
func LoneEditor(eds []EditorCandidate) *EditorCandidate {
	var editors []EditorCandidate
	hasGhostty := false
	for _, e := range eds {
		switch e.Binary {
		case WorkspaceSentinel:
			hasGhostty = true
		case SkipSentinel:
			// ignore
		default:
			editors = append(editors, e)
		}
	}
	if len(editors) == 1 && !hasGhostty {
		c := editors[0]
		return &c
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
