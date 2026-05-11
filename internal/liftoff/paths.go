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
	Root      string // e.g. /Users/acs/liftoff
	Master    string // e.g. /Users/acs/liftoff/liftoff-app-master
	GtabDir   string // e.g. /Users/acs/.config/gtab
	MainBranch string
}

// DefaultLayout resolves Layout via three layers, in order:
//
//	1. env override (KIT_ROOT etc.)
//	2. config.toml [settings] block (kit setup writes this)
//	3. built-in default
//
// Config load failures fall through to env + built-ins so kit always
// boots.
func DefaultLayout() Layout {
	home, _ := os.UserHomeDir()
	settings := Settings{}
	if c, err := LoadConfig(); err == nil && c != nil {
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
	return "", fmt.Errorf("worktree not found: %s", path)
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
