package liftoff

import (
	"fmt"
	"os"
)

// Layouts and launch commands belong to Kit, not either terminal runtime.
func BuiltinWorkspaceLayouts() map[string]WorkspaceLayout {
	return map[string]WorkspaceLayout{
		"default":  {Tabs: []string{"shell", "logs"}},
		"simple":   {Tabs: []string{"shell"}},
		"detailed": {Tabs: []string{"shell", "frontend", "backend", "celery", "logs"}},
		"ai":       {Tabs: []string{"shell", "claude", "codex", "gemini", "logs"}},
	}
}

func ResolveWorkspaceLayout(name string) (WorkspaceLayout, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return WorkspaceLayout{}, err
	}
	if name == "" {
		name = os.Getenv("KIT_WORKSPACE_LAYOUT")
		if name == "" {
			name = os.Getenv("KIT_HERDR_LAYOUT")
		}
		if name == "" {
			name = cfg.Settings.WorkspaceLayout
		}
		if name == "" {
			name = cfg.Settings.HerdrLayout
		}
		if name == "" {
			name = "default"
		}
	}
	layouts := BuiltinWorkspaceLayouts()
	for key, layout := range cfg.Layouts {
		layouts[key] = layout
	}
	layout, ok := layouts[name]
	if !ok || len(layout.Tabs) == 0 {
		return WorkspaceLayout{}, fmt.Errorf("unknown terminal layout %q", name)
	}
	return layout, nil
}

func workspaceTabCommand(name, tabName string) string {
	switch tabName {
	case "logs":
		bin, err := ResolvedExecutable()
		if err != nil || bin == "" {
			bin = "kit"
		}
		return shellQuote(bin) + " log " + shellQuote(name) + " --wait"
	case "frontend":
		return herdrCombinedTailCommand(name, SvcApp, SvcAdmin)
	case "backend":
		return herdrCombinedTailCommand(name, SvcAPI, SvcAdminBE)
	case "celery":
		return herdrTailCommand(name, SvcCelery)
	case "claude", "codex", "gemini":
		return fmt.Sprintf("if command -v %s >/dev/null 2>&1; then exec %s; else printf 'Kit: %s is not installed\\n'; exec \"${SHELL:-/bin/sh}\"; fi", tabName, tabName, tabName)
	default:
		return ""
	}
}
