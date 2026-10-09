package liftoff

import (
	"fmt"
	"os"
	"path/filepath"
)

// hasSavedManagedWorkspace reports whether a persisted workspace mapping
// authorizes managed-workspace cleanup. Only persisted mappings authorize
// cleanup: resolving an unmapped workspace by label could close a session
// the user created by hand. Covers both backends so switching the selected
// backend never makes old resource ownership disappear.
func hasSavedManagedWorkspace(name string) bool {
	c, err := LoadConfig()
	if err != nil {
		return false
	}
	meta := c.Worktrees[name]
	return meta.RexID != "" || meta.HerdrID != "" || meta.HerdrSpace != ""
}

// resolveCleanupDB decides which database cleanup may drop for name.
//
// The persisted WorktreeMeta.DatabaseName is exact when set (recorded at
// creation); otherwise the name is derived via DBName and every other config
// record is checked for a competing claim — DBName collides for names that
// differ only by "-" vs "_". A disputed drop is an error: the caller must
// fail and retain recovery metadata so a human decides.
//
// skip=true means the worktree provably uses a different database (its env
// still points elsewhere, e.g. it never cloned one): there is nothing owned
// to drop, so the caller continues without dropping. Skipping leaks nothing
// it could destroy; guessing would.
//
// The returned value is ownership evidence, never logged.
func resolveCleanupDB(name, worktreePath string) (db string, skip bool, err error) {
	cfg, err := LoadConfig()
	if err != nil {
		return "", false, err
	}
	meta := cfg.Worktrees[name]
	candidate := meta.DatabaseName
	if candidate == "" {
		candidate = DBName(name)
	} else {
		if candidate == "liftoff" {
			return "", false, fmt.Errorf("refusing to drop shared database %q", candidate)
		}
		// Exact persisted mapping: authoritative, but still refuse when
		// another record claims the same database (duplicate after a
		// rename or adopt must not destroy a live database).
		for other, otherMeta := range cfg.Worktrees {
			if other == name {
				continue
			}
			claimed := otherMeta.DatabaseName
			if claimed == "" {
				claimed = DBName(other)
			}
			if claimed == candidate {
				return "", false, fmt.Errorf("database %s belongs to worktree %q; not dropping it with %q", candidate, other, name)
			}
		}
		return candidate, false, nil
	}
	for other, otherMeta := range cfg.Worktrees {
		if other == name {
			continue
		}
		claimed := otherMeta.DatabaseName
		if claimed == "" {
			claimed = DBName(other)
		}
		if claimed == candidate {
			return "", false, fmt.Errorf("database %s belongs to worktree %q; not dropping it with %q", candidate, other, name)
		}
	}
	// Legacy proof: the checkout's own env must still point at the derived
	// name. Without it the name is only a label guess.
	if worktreePath != "" {
		envDB, found, envErr := DBNameFromEnv(worktreePath)
		if envErr == nil && !found {
			// Kit writes the key only when it clones a database. An env
			// file without it means the checkout uses the app default.
			if _, statErr := os.Stat(filepath.Join(worktreePath, "backend", ".env")); statErr == nil {
				return "", true, nil
			}
		}
		if envErr != nil || !found || envDB == "" {
			return "", false, fmt.Errorf("cannot prove database ownership for %q; refusing to guess", name)
		}
		if envDB != candidate {
			return "", true, nil
		}
		// Preserve the proof before cleanup can remove the checkout and env.
		if err := WithConfigLock(func(c *Config) error {
			m := c.Worktrees[name]
			if m.DatabaseName == "" {
				m.DatabaseName = candidate
				c.Worktrees[name] = m
			}
			return nil
		}); err != nil {
			return "", false, fmt.Errorf("persist database ownership for %q: %w", name, err)
		}
	} else {
		return "", false, fmt.Errorf("cannot prove database ownership for %q; refusing to guess", name)
	}
	if candidate == "liftoff" {
		return "", false, fmt.Errorf("refusing to drop shared database %q", candidate)
	}
	return candidate, false, nil
}

// verifyWorktreeOwnership refuses to remove the checkout at path when git has
// a live worktree registered there for a different worktree identity. A stale
// plan path must never become permission to `worktree remove --force` someone
// else's checkout. An unregistered path is left to RemoveWorktree, which
// already refuses to delete existing folders git does not know about.
func (l Layout) verifyWorktreeOwnership(path, name string) error {
	wts, err := l.ListWorktrees()
	if err != nil {
		return err
	}
	branch := ""
	if cfg, err := LoadConfig(); err == nil {
		branch = cfg.Worktrees[name].Branch
	}
	if branch == "" {
		branch = name
	}
	for _, wt := range wts {
		if cleanPath(wt.Path) != cleanPath(path) {
			continue
		}
		if wt.IsMaster(l) {
			return fmt.Errorf("refusing to remove the master checkout %s", path)
		}
		if wt.Name() == name || (wt.Branch != "" && wt.Branch == branch) {
			return nil
		}
		return fmt.Errorf("worktree path %s belongs to %q; not removing it with %q", path, wt.Name(), name)
	}
	return nil
}
