package liftoff

import (
	"os"
	"os/exec"
	"text/template"
)

// gtabTemplate replicates the AppleScript layout from the original zshrc:
// 1 window, 4 tabs:
//   - tab 1: worktree root, single pane
//   - tab 2: frontend/app + frontend/admin split
//   - tab 3: backend + backend split
//   - tab 4: backend (celery)
const gtabTemplate = `tell application "Ghostty"
    activate

    set cfg1 to new surface configuration
    set initial working directory of cfg1 to "{{.Worktree}}"
    set win to new window with configuration cfg1
    set p1 to focused terminal of selected tab of win
    perform action "set_tab_title:{{.TabTitle}}" on p1

    set cfg2 to new surface configuration
    set initial working directory of cfg2 to "{{.Worktree}}/frontend/app"
    set newtab1 to new tab in win with configuration cfg2
    set p2 to focused terminal of newtab1
    perform action "set_tab_title:frontend" on p2
    set cfgSplit2 to new surface configuration
    set initial working directory of cfgSplit2 to "{{.Worktree}}/frontend/admin"
    set p2b to split p2 direction right with configuration cfgSplit2

    set cfg3 to new surface configuration
    set initial working directory of cfg3 to "{{.Worktree}}/backend"
    set newtab2 to new tab in win with configuration cfg3
    set p3 to focused terminal of newtab2
    perform action "set_tab_title:backend" on p3
    set cfgSplit3 to new surface configuration
    set initial working directory of cfgSplit3 to "{{.Worktree}}/backend"
    set p3b to split p3 direction right with configuration cfgSplit3

    set cfg4 to new surface configuration
    set initial working directory of cfg4 to "{{.Worktree}}/backend"
    set newtab3 to new tab in win with configuration cfg4
    set p4 to focused terminal of newtab3
    perform action "set_tab_title:celery" on p4
end tell
`

type gtabData struct {
	Name     string
	TabTitle string // Name with emoji prefix when available
	Worktree string
}

// WriteGtab generates the AppleScript file for a worktree.
// Creates the gtab dir if missing.
func (l Layout) WriteGtab(name, worktree string) (string, error) {
	if err := os.MkdirAll(l.GtabDir, 0o755); err != nil {
		return "", err
	}
	path := l.GtabFile(name)
	tmpl, err := template.New("gtab").Parse(gtabTemplate)
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return "", err
	}
	defer f.Close()
	tabTitle := name
	if e := EmojiFor(name); e != "" {
		tabTitle = e + " " + name
	}
	if err := tmpl.Execute(f, gtabData{Name: name, TabTitle: tabTitle, Worktree: worktree}); err != nil {
		return "", err
	}
	return path, nil
}

// RemoveGtab deletes both the new-format and legacy AppleScript files (if present).
// Returns nil if neither exists.
func (l Layout) RemoveGtab(name string) error {
	for _, path := range []string{l.GtabFile(name), l.LegacyGtabFile(name)} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// LegacyGtabFile returns the old zshrc-era path (~/.config/gtab/liftoff-<name>.applescript).
// Used so worktrees created before kit existed are still launchable.
func (l Layout) LegacyGtabFile(name string) string {
	return l.GtabFile("liftoff-" + name)
}

// LaunchGtab runs `gtab <name>` (or `osascript <path>` if gtab is not on PATH).
// Falls back to the legacy filename if the new-format file is missing.
func (l Layout) LaunchGtab(name string) error {
	if _, err := exec.LookPath("gtab"); err == nil {
		// gtab CLI looks up by stem, so try both.
		if _, err := os.Stat(l.GtabFile(name)); err == nil {
			return exec.Command("gtab", name).Start()
		}
		if _, err := os.Stat(l.LegacyGtabFile(name)); err == nil {
			return exec.Command("gtab", "liftoff-"+name).Start()
		}
	}
	path := l.GtabFile(name)
	if _, err := os.Stat(path); err != nil {
		path = l.LegacyGtabFile(name)
	}
	return exec.Command("osascript", path).Start()
}

// HasGtab returns true if either the new-format or legacy AppleScript exists.
func (l Layout) HasGtab(name string) bool {
	if _, err := os.Stat(l.GtabFile(name)); err == nil {
		return true
	}
	if _, err := os.Stat(l.LegacyGtabFile(name)); err == nil {
		return true
	}
	return false
}

// EnsureGtabDir is a no-op convenience to make the dir.
func (l Layout) EnsureGtabDir() error {
	return os.MkdirAll(l.GtabDir, 0o755)
}
