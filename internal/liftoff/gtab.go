package liftoff

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// GtabLayout selects between the simple (default) and detailed gtab
// templates. Simple = 2 tabs (shell + combined tail). Detailed = 5 tabs
// (shell, frontend split, backend split, celery, combined logs).
type GtabLayout string

const (
	GtabSimple   GtabLayout = "simple"
	GtabDetailed GtabLayout = "detailed"
)

// WriteGtab generates the AppleScript file using the simple layout.
// Use WriteGtabLayout for the detailed variant.
func (l Layout) WriteGtab(name, worktree string) (string, error) {
	return l.WriteGtabLayout(name, worktree, GtabSimple)
}

// WriteGtabLayout generates the AppleScript file for a worktree using
// the chosen layout. Creates the gtab dir if missing.
func (l Layout) WriteGtabLayout(name, worktree string, layout GtabLayout) (string, error) {
	if err := os.MkdirAll(l.GtabDir, 0o755); err != nil {
		return "", err
	}
	layoutName := "default"
	if layout == GtabDetailed {
		layoutName = "detailed"
	}
	wsLayout, err := ResolveWorkspaceLayout(layoutName)
	if err != nil {
		return "", err
	}
	body := generateGtabScript(name, worktree, wsLayout)
	path := l.GtabFile(name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func appleQuote(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return "\"" + s + "\""
}

func generateGtabScript(name, worktree string, wsLayout WorkspaceLayout) string {
	var b strings.Builder
	b.WriteString("tell application \"Ghostty\"\n    activate\n\n")
	b.WriteString("    set cfg1 to new surface configuration\n")
	fmt.Fprintf(&b, "    set initial working directory of cfg1 to %s\n", appleQuote(worktree))
	b.WriteString("    set win to new window with configuration cfg1\n")
	b.WriteString("    set p1 to focused terminal of selected tab of win\n")
	fmt.Fprintf(&b, "    perform action %s on p1\n\n", appleQuote("set_tab_title:"+HerdrShellTabLabel(name)))
	for idx, tab := range wsLayout.Tabs {
		if tab == "shell" && idx == 0 {
			continue
		}
		cfgNum := idx + 1
		panes := WorkspaceTabPanes(name, worktree, tab)
		if len(panes) == 0 {
			continue
		}
		fmt.Fprintf(&b, "    set cfg%d to new surface configuration\n", cfgNum)
		fmt.Fprintf(&b, "    set initial working directory of cfg%d to %s\n", cfgNum, appleQuote(panes[0].Cwd))
		if panes[0].Command != "" {
			fmt.Fprintf(&b, "    set command of cfg%d to %s\n", cfgNum, appleQuote(panes[0].Command))
			fmt.Fprintf(&b, "    set wait after command of cfg%d to true\n", cfgNum)
		}
		fmt.Fprintf(&b, "    set newtab%d to new tab in win with configuration cfg%d\n", idx, cfgNum)
		fmt.Fprintf(&b, "    set p%d to focused terminal of newtab%d\n", cfgNum, idx)
		fmt.Fprintf(&b, "    perform action %s on p%d\n", appleQuote("set_tab_title:"+tab), cfgNum)
		for s := 1; s < len(panes); s++ {
			fmt.Fprintf(&b, "    set cfgSplit%d to new surface configuration\n", cfgNum)
			fmt.Fprintf(&b, "    set initial working directory of cfgSplit%d to %s\n", cfgNum, appleQuote(panes[s].Cwd))
			if panes[s].Command != "" {
				fmt.Fprintf(&b, "    set command of cfgSplit%d to %s\n", cfgNum, appleQuote(panes[s].Command))
				fmt.Fprintf(&b, "    set wait after command of cfgSplit%d to true\n", cfgNum)
			}
			fmt.Fprintf(&b, "    set p%db to split p%d direction right with configuration cfgSplit%d\n", cfgNum, cfgNum, cfgNum)
		}
		b.WriteString("\n")
	}
	b.WriteString("end tell\n")
	return b.String()
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

// kitBinaryPath returns an absolute path to the current kit binary,
// suitable for embedding in AppleScript. Ghostty launches gtab tabs
// without sourcing ~/.zshrc / .bashrc, so a bare `kit` invocation
// fails with "command not found" even when kit is on the user's
// interactive PATH. Falls back to "kit" if resolution fails — the
// gtab will still work for users whose system PATH covers it.
func kitBinaryPath() string {
	if exe, err := ResolvedExecutable(); err == nil {
		return exe
	}
	return "kit"
}
