package liftoff

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Layout holds resolved Liftoff filesystem paths. All values are absolute.
//
// Defaults match the canonical Liftoff dev layout:
//
//	~/liftoff/                       -- root
//	~/liftoff/liftoff-app-master/    -- master repo
//	~/liftoff/<feature>/             -- feature worktrees (clean naming)
//	~/.config/gtab/<feature>.applescript
//
// Override via env vars:
//
//	KIT_ROOT          -- override ~/liftoff
//	KIT_MASTER_DIR    -- override liftoff-app-master subdir name
//	KIT_GTAB_DIR      -- override ~/.config/gtab
type Layout struct {
	Root       string // e.g. /Users/acs/liftoff
	Master     string // e.g. /Users/acs/liftoff/liftoff-app-master
	GtabDir    string // e.g. /Users/acs/.config/gtab
	MainBranch string
}

// DefaultLayout resolves Layout via three layers, in order:
//
//  1. env override (KIT_ROOT etc.)
//  2. config.toml [settings] block (kit setup writes this)
//  3. built-in default
//
// Config load failures fall through to env + built-ins so kit always
// boots.
func DefaultLayout() Layout {
	home, _ := os.UserHomeDir()
	settings := Settings{}
	// Missing file returns nil error, so a non-nil error here is a real parse
	// failure — warn instead of silently falling back to defaults.
	if c, err := LoadConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "kit: warning: %s unreadable (%v); using defaults\n", ConfigPath(), err)
	} else if c != nil {
		settings = c.Settings
	}

	root := resolve("KIT_ROOT", settings.Root, filepath.Join(home, "liftoff"))
	masterName := resolve("KIT_MASTER_DIR", settings.MasterDir, "liftoff-app-master")
	gtabDir := resolve("KIT_GTAB_DIR", settings.GtabDir, filepath.Join(home, ".config", "gtab"))
	mainBranch := resolve("KIT_MAIN_BRANCH", settings.MainBranch, "master")

	return Layout{
		Root:       root,
		Master:     filepath.Join(root, masterName),
		GtabDir:    gtabDir,
		MainBranch: mainBranch,
	}
}

// resolve picks the first non-empty value from: env override, config-file
// setting, fallback.
func resolve(envKey, configValue, fallback string) string {
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	if configValue != "" {
		return configValue
	}
	return fallback
}

// WorktreePath returns the canonical worktree path for a feature name.
// Uses clean naming: ~/liftoff/<name> (no liftoff- prefix).
func (l Layout) WorktreePath(name string) string {
	return filepath.Join(l.Root, name)
}

// LegacyWorktreePath returns the legacy ~/liftoff/liftoff-<name> path.
// Kept for back-compat detection in lineup/wash.
func (l Layout) LegacyWorktreePath(name string) string {
	return filepath.Join(l.Root, "liftoff-"+name)
}

// ResolveWorktreePath returns the on-disk dir for a kit: layout.Master for
// "master", the canonical WorktreePath, or the legacy fallback. Errors
// when neither exists for a non-master name.
func (l Layout) ResolveWorktreePath(name string) (string, error) {
	if name == "master" {
		return l.Master, nil
	}
	path := l.WorktreePath(name)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	legacy := l.LegacyWorktreePath(name)
	if _, err := os.Stat(legacy); err == nil {
		return legacy, nil
	}
	// Recovery for a "lost" worktree — one moved or renamed out from under
	// kit's canonical/legacy layout that kit's config or git still tracks.
	// These fallbacks run only after the path lookups above have failed, so
	// the happy path is unchanged.
	if cfg, err := LoadConfig(); err == nil {
		if m, ok := cfg.Worktrees[name]; ok && m.Path != "" {
			if _, err := os.Stat(m.Path); err == nil {
				return m.Path, nil
			}
		}
	}
	if p := l.gitWorktreePath(name); p != "" {
		return p, nil
	}
	return "", fmt.Errorf("worktree not found: %s", path)
}

// gitWorktreePath asks git which checkout corresponds to `name`, to recover a
// worktree moved or renamed out from under kit's canonical layout. Matches, in
// order: a checkout whose directory basename equals name, one whose branch
// equals name, then one whose branch equals the branch kit recorded for name.
// Returns "" when git can't help (not a repo, git missing, or no live match).
func (l Layout) gitWorktreePath(name string) string {
	wts, err := l.ListWorktrees()
	if err != nil {
		return ""
	}
	exists := func(p string) bool {
		if p == "" {
			return false
		}
		_, err := os.Stat(p)
		return err == nil
	}
	set := NewWorktreeSet(wts)
	if wt, ok := set.Find(name, name, ""); ok && exists(wt.Path) {
		return wt.Path
	}
	if cfg, err := LoadConfig(); err == nil {
		if m, ok := cfg.Worktrees[name]; ok && m.Branch != "" {
			if wt, ok := set.Find("", m.Branch, ""); ok && exists(wt.Path) {
				return wt.Path
			}
		}
	}
	return ""
}

// GtabFile returns the AppleScript path for a feature.
func (l Layout) GtabFile(name string) string {
	return filepath.Join(l.GtabDir, name+".applescript")
}

// EnvFiles describes the env files that get copied from master into a worktree.
// Pairs are (relative path under repo root). Same path used on both sides.
var EnvFiles = []string{
	".env",
	"backend/.env",
	"frontend/env/.env.local",
	"frontend/admin/env/.env.local",
}

// DBName returns the per-feature postgres database name.
// Replaces dashes with underscores; postgres dislikes dashes in identifiers.
func DBName(featureName string) string {
	return "liftoff_" + strings.ReplaceAll(featureName, "-", "_")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
