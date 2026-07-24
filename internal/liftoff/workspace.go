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
	Name          string
	Binary        string   // CLI binary name (preferred when on PATH unless PreferApp or AppOnly)
	App           string   // primary .app bundle name (e.g. "Zed.app") for `open -a`
	AlternateApps []string // fallback bundle names for the same picker entry
	Desc          string
	Installed     bool
	UseOpen       bool // true when launch is via `open -a App` not `binary`
	AppOnly       bool // always launch via the .app bundle; ignore any PATH binary
	PreferApp     bool // prefer an installed bundle, but fall back to the PATH binary
}

// editorDefs is the canonical candidate list, ordered by preference.
func editorDefs() []EditorCandidate {
	return []EditorCandidate{
		{Name: "Hangar", Binary: "hangar", App: "Hangar Dev.app", AlternateApps: []string{"Hangar.app"}, Desc: "open in Hangar", PreferApp: true},
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
// to the front, followed by config.toml's settings.editor. A custom preferred
// editor is resolved via PATH.
//
// When the Ghostty.app bundle is present, a synthetic "Ghostty workspace"
// candidate (Binary == WorkspaceSentinel) is appended so a single picker can
// offer both editors and the dev-workspace flow.
func InstalledEditors() []EditorCandidate {
	defs := editorDefs()
	if preferred, source := preferredEditor(); preferred != "" {
		defs = promoteEditor(defs, preferred, source)
	}
	out := make([]EditorCandidate, 0, len(defs))
	seen := make(map[string]bool, len(defs))
	for _, e := range defs {
		c := e
		installedApp := firstInstalledApp(c)
		_, binaryErr := exec.LookPath(c.Binary)
		binaryInstalled := c.Binary != "" && binaryErr == nil
		if installedApp != "" {
			c.App = installedApp
			c.Installed = true
			c.UseOpen = c.AppOnly || c.PreferApp || !binaryInstalled
		} else if !c.AppOnly && binaryInstalled {
			// A known editor's CLI is sufficient even when its application bundle
			// is outside the standard /Applications locations.
			c.Installed = true
			c.UseOpen = false
		} else {
			continue
		}
		if !seen[c.Binary] {
			out = append(out, c)
			seen[c.Binary] = true
		}
	}
	// Zed is always offered, even if detection above missed it.
	out = ensureZedOffered(out)
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
// name. It honors each known editor's bundle/CLI preference and falls back to
// a matching PATH binary for custom names. Returns nil when not found.
func ResolveEditor(name string) *EditorCandidate {
	for _, def := range editorDefs() {
		if def.Binary == name || strings.EqualFold(def.Name, name) {
			c := def
			if app := firstInstalledApp(c); app != "" && (c.AppOnly || c.PreferApp) {
				c.App = app
				c.Installed = true
				c.UseOpen = true
				return &c
			}
			if !c.AppOnly {
				if _, err := exec.LookPath(c.Binary); err == nil {
					c.Installed = true
					return &c
				}
			}
			if app := firstInstalledApp(c); app != "" {
				c.App = app
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

// preferredEditor returns the configured editor and where the preference came
// from. Environment always wins over the durable config setting.
func preferredEditor() (string, string) {
	if value := os.Getenv("KIT_EDITOR"); value != "" {
		return value, "from $KIT_EDITOR"
	}
	if c, err := LoadConfig(); err == nil && c.Settings.Editor != "" {
		return c.Settings.Editor, "from config.toml"
	}
	return "", ""
}

// promoteEditor moves a known editor to the front without duplicating its
// picker entry. Unknown configured editors are allowed when their CLI is on
// PATH and are prepended as a generic candidate.
func promoteEditor(defs []EditorCandidate, preferred, source string) []EditorCandidate {
	for i, def := range defs {
		if def.Binary != preferred && !strings.EqualFold(def.Name, preferred) {
			continue
		}
		def.Desc = source
		out := make([]EditorCandidate, 0, len(defs))
		out = append(out, def)
		out = append(out, defs[:i]...)
		out = append(out, defs[i+1:]...)
		return out
	}
	return append([]EditorCandidate{{Name: preferred, Binary: preferred, Desc: source}}, defs...)
}

// firstInstalledApp returns the first installed bundle for a candidate. The
// primary name wins, allowing Hangar Dev and stable Hangar to share one entry.
func firstInstalledApp(c EditorCandidate) string {
	apps := append([]string{c.App}, c.AlternateApps...)
	for _, app := range apps {
		if app != "" && appBundleExists(app) {
			return app
		}
	}
	return ""
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
