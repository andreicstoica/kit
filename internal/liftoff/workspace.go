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

// HerdrSentinel is the synthetic EditorCandidate.Binary value marking the
// "herdr agent multiplexer" target. Callers route it to OpenHerdr instead
// of LaunchEditor. Like the Ghostty workspace, herdr is a distinct intent
// (not an editor), so its presence keeps the picker visible.
const HerdrSentinel = "__herdr__"

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
		// Claude Code desktop's GUI is Claude.app, which registers public.folder
		// as an Editor doc type — so `open -a Claude.app <path>` opens the
		// worktree as a Claude Code session. AppOnly forces that bundle launch:
		// the `claude` CLI starts a terminal session, not the GUI, so we never
		// want the PATH-binary path. Binary stays the `-e` invocation token.
		{Name: "Claude Code desktop", Binary: "claude-code-desktop", App: "Claude.app", Desc: "open in Claude Code desktop", AppOnly: true},
	}
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
	if appBundleExists("Ghostty.app") {
		out = append(out, EditorCandidate{
			Name:      "Ghostty (pick layout next)",
			Binary:    WorkspaceSentinel,
			App:       "Ghostty.app",
			Desc:      "dev workspace — simple (2 tabs) or detailed (5 tabs)",
			Installed: true,
		})
	}
	// herdr is an agent multiplexer: one persistent window holding every
	// worktree. Offered only when the CLI is on PATH; opening a worktree
	// adds a workspace to the running herdr rather than launching a window.
	if _, err := exec.LookPath("herdr"); err == nil {
		out = append(out, EditorCandidate{
			Name:      "herdr (agent multiplexer)",
			Binary:    HerdrSentinel,
			Desc:      "add a workspace to the running herdr",
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

// LoneEditor returns the single installed editor when no picker is needed.
// Returns nil when there are zero editors, two or more editors, or any number
// plus the Ghostty or herdr target (each is a distinct intent, so the picker
// still appears). The SkipSentinel candidates are ignored.
func LoneEditor(eds []EditorCandidate) *EditorCandidate {
	var editors []EditorCandidate
	hasGhostty := false
	hasHerdr := false
	for _, e := range eds {
		switch e.Binary {
		case WorkspaceSentinel:
			hasGhostty = true
		case HerdrSentinel:
			hasHerdr = true
		case SkipSentinel:
			// ignore
		default:
			editors = append(editors, e)
		}
	}
	if len(editors) == 1 && !hasGhostty && !hasHerdr {
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

// OpenHerdr adds a workspace for the worktree to the running herdr agent
// multiplexer via its socket CLI, focuses it, and records the worktree as
// last-used. herdr runs a persistent server holding one window for every
// worktree, so kit adds to that instance rather than launching its own.
//
// The server must already be running: creating the workspace in a headless
// server the user can't see would be worse than a clear hint, so we precheck
// `herdr status server` and point at `herdr` when it's down.
//
// Invocation matches herdr 0.7's socket CLI
// (`herdr workspace create --cwd <path> --label <text> --focus`); this is the
// one spot to adjust if a future herdr changes the surface.
func OpenHerdr(name, path string) error {
	if !herdrServerRunning() {
		return fmt.Errorf("herdr server not running — start it first with `herdr` " +
			"(or `brew services start herdr`), then retry")
	}
	label := name
	if e := EmojiFor(name); e != "" {
		label = e + " " + name
	}
	// `workspace create` emits a JSON result on stdout — discard it (the caller
	// prints a clean confirmation). Keep stderr for real failures.
	cmd := exec.Command("herdr", "workspace", "create", "--cwd", path, "--label", label, "--focus")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("herdr workspace create: %w", err)
	}
	TouchLastUsedName(name)
	return nil
}

// herdrServerRunning reports whether the herdr server is up. `herdr status
// server` always exits 0, printing "status: running" or "status: not
// running" — so we parse the output rather than the exit code.
func herdrServerRunning() bool {
	out, err := exec.Command("herdr", "status", "server").CombinedOutput()
	if err != nil {
		return false
	}
	return herdrServerUp(string(out))
}

// herdrServerUp reports whether `herdr status server` output indicates a
// running server. Split from the exec call so it's unit-testable.
func herdrServerUp(statusOutput string) bool {
	return strings.Contains(statusOutput, "status: running")
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
