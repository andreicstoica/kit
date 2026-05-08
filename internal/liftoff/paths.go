package liftoff

import (
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

// DefaultLayout resolves Layout from $HOME and env overrides.
func DefaultLayout() Layout {
	home, _ := os.UserHomeDir()

	root := envOr("KIT_ROOT", filepath.Join(home, "liftoff"))
	masterName := envOr("KIT_MASTER_DIR", "liftoff-app-master")
	gtabDir := envOr("KIT_GTAB_DIR", filepath.Join(home, ".config", "gtab"))

	return Layout{
		Root:       root,
		Master:     filepath.Join(root, masterName),
		GtabDir:    gtabDir,
		MainBranch: envOr("KIT_MAIN_BRANCH", "master"),
	}
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
