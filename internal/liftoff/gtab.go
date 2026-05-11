package liftoff

import (
	"os"
	"os/exec"
	"text/template"
)

// gtabTemplate lays out a Ghostty workspace per worktree:
//   - tab 1: worktree root, bare shell (no auto-command)
//   - tab 2: frontend/app + frontend/admin split, each pane auto-tails the
//     matching kit-managed log
//   - tab 3: backend (api) + backend (admin_be) split, each tails its log
//   - tab 4: backend, tails celery worker log
//
// Each auto-tail pane uses `tail -F` so the surface waits if the log
// hasn't been created yet (kit play not run). `wait after command true`
// keeps the surface open at a shell prompt once tail exits (Ctrl-C).
const gtabTemplate = `tell application "Ghostty"
    activate

    set cfg1 to new surface configuration
    set initial working directory of cfg1 to "{{.Worktree}}"
    set win to new window with configuration cfg1
    set p1 to focused terminal of selected tab of win
    perform action "set_tab_title:{{.TabTitle}}" on p1

    set cfg2 to new surface configuration
    set initial working directory of cfg2 to "{{.Worktree}}/frontend/app"
    set command of cfg2 to "tail -F {{.AppLog}}"
    set wait after command of cfg2 to true
    set newtab1 to new tab in win with configuration cfg2
    set p2 to focused terminal of newtab1
    perform action "set_tab_title:frontend" on p2
    set cfgSplit2 to new surface configuration
    set initial working directory of cfgSplit2 to "{{.Worktree}}/frontend/admin"
    set command of cfgSplit2 to "tail -F {{.AdminLog}}"
    set wait after command of cfgSplit2 to true
    set p2b to split p2 direction right with configuration cfgSplit2

    set cfg3 to new surface configuration
    set initial working directory of cfg3 to "{{.Worktree}}/backend"
    set command of cfg3 to "tail -F {{.APILog}}"
    set wait after command of cfg3 to true
    set newtab2 to new tab in win with configuration cfg3
    set p3 to focused terminal of newtab2
    perform action "set_tab_title:backend" on p3
    set cfgSplit3 to new surface configuration
    set initial working directory of cfgSplit3 to "{{.Worktree}}/backend"
    set command of cfgSplit3 to "tail -F {{.AdminBELog}}"
    set wait after command of cfgSplit3 to true
    set p3b to split p3 direction right with configuration cfgSplit3

    set cfg4 to new surface configuration
    set initial working directory of cfg4 to "{{.Worktree}}/backend"
    set command of cfg4 to "tail -F {{.CeleryLog}}"
    set wait after command of cfg4 to true
    set newtab3 to new tab in win with configuration cfg4
    set p4 to focused terminal of newtab3
    perform action "set_tab_title:celery" on p4
end tell
`

type gtabData struct {
	Name       string
	TabTitle   string // Name with emoji prefix when available
	Worktree   string
	AppLog     string
	AdminLog   string
	APILog     string
	AdminBELog string
	CeleryLog  string
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
	appLog, _ := LogFile(name, string(SvcApp))
	adminLog, _ := LogFile(name, string(SvcAdmin))
	apiLog, _ := LogFile(name, string(SvcAPI))
	adminBELog, _ := LogFile(name, string(SvcAdminBE))
	celeryLog, _ := LogFile(name, string(SvcCelery))
	data := gtabData{
		Name:       name,
		TabTitle:   tabTitle,
		Worktree:   worktree,
		AppLog:     appLog,
		AdminLog:   adminLog,
		APILog:     apiLog,
		AdminBELog: adminBELog,
		CeleryLog:  celeryLog,
	}
	if err := tmpl.Execute(f, data); err != nil {
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
