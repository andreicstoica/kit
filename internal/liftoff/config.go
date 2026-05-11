package liftoff

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	configSchema   = 2
	configFileName = "config.toml"
	maxSlot        = 99
)

// Settings is the durable, user-facing config block. Each field maps to an
// existing env-var override; env wins at read time when set. Persisted by
// `kit setup` and editable by hand.
type Settings struct {
	Root        string `toml:"root,omitempty"`
	MasterDir   string `toml:"master_dir,omitempty"`
	GtabDir     string `toml:"gtab_dir,omitempty"`
	MainBranch  string `toml:"main_branch,omitempty"`
	PyVenv      string `toml:"py_venv,omitempty"`
	Editor      string `toml:"editor,omitempty"`
	LiftoffRepo string `toml:"liftoff_repo,omitempty"`
}

// WorktreeMeta is the persisted record for one worktree in config.toml.
//
// Branch, Path, and Adopted were added in schema 2. Older schema-1 files
// load with empty values and are migrated transparently on next write.
type WorktreeMeta struct {
	Slot     int       `toml:"slot"`
	Created  time.Time `toml:"created"`
	LastUsed time.Time `toml:"last_used"`
	Branch   string    `toml:"branch,omitempty"`  // actual git branch (may differ from key)
	Path     string    `toml:"path,omitempty"`    // worktree path (for adoption troubleshooting)
	Adopted  bool      `toml:"adopted,omitempty"` // true when added via kit adopt (vs kit design)
}

// Config is the on-disk shape of ~/.config/kit/config.toml.
type Config struct {
	Schema    int                     `toml:"schema"`
	Settings  Settings                `toml:"settings,omitempty"`
	Worktrees map[string]WorktreeMeta `toml:"worktrees,omitempty"`
}

// State is the legacy name for Config; kept as a type alias so callers
// don't have to change in lockstep. Prefer Config in new code.
type State = Config

// ConfigPath returns ~/.config/kit/config.toml (honoring KIT_STATE_DIR).
func ConfigPath() string {
	return filepath.Join(configDir(), configFileName)
}

// StatePath is the legacy name for ConfigPath.
func StatePath() string { return ConfigPath() }

func configDir() string {
	if v := os.Getenv("KIT_STATE_DIR"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "kit")
}

// LoadConfig reads config.toml. Missing file returns an empty Config.
func LoadConfig() (*Config, error) {
	c := &Config{Schema: configSchema, Worktrees: map[string]WorktreeMeta{}}
	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return c, nil
		}
		return nil, err
	}
	if _, err := toml.Decode(string(data), c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", ConfigPath(), err)
	}
	if c.Worktrees == nil {
		c.Worktrees = map[string]WorktreeMeta{}
	}
	if c.Schema == 0 {
		c.Schema = configSchema
	}
	return c, nil
}

// LoadState is the legacy name for LoadConfig.
func LoadState() (*State, error) { return LoadConfig() }

// Save writes config.toml atomically.
func (c *Config) Save() error {
	path := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if c.Schema == 0 {
		c.Schema = configSchema
	}
	enc := toml.NewEncoder(f)
	if err := enc.Encode(c); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// AllocateSlot picks the lowest unused slot ≥ 1 and persists it.
// portsFree is called per candidate slot to verify all ports in the band are
// bindable; pass nil to skip the check (tests).
func (c *Config) AllocateSlot(name string, portsFree func(slot int) bool) (int, error) {
	if existing, ok := c.Worktrees[name]; ok && existing.Slot > 0 {
		return existing.Slot, nil
	}
	used := map[int]bool{0: true}
	for _, m := range c.Worktrees {
		if m.Slot > 0 {
			used[m.Slot] = true
		}
	}
	for slot := 1; slot <= maxSlot; slot++ {
		if used[slot] {
			continue
		}
		if portsFree != nil && !portsFree(slot) {
			continue
		}
		now := time.Now().UTC()
		existing := c.Worktrees[name]
		existing.Slot = slot
		if existing.Created.IsZero() {
			existing.Created = now
		}
		existing.LastUsed = now
		c.Worktrees[name] = existing
		return slot, nil
	}
	return 0, fmt.Errorf("no free slot ≤ %d (you have a lot of worktrees!)", maxSlot)
}

// FreeSlot removes a worktree's record. Idempotent.
func (c *Config) FreeSlot(name string) {
	delete(c.Worktrees, name)
}

// TouchLastUsed updates the LastUsed timestamp for a worktree.
// No-op if the worktree is absent.
func (c *Config) TouchLastUsed(name string) {
	m, ok := c.Worktrees[name]
	if !ok {
		return
	}
	m.LastUsed = time.Now().UTC()
	c.Worktrees[name] = m
}

// SortedNames returns worktree names sorted by LastUsed descending,
// then by Slot ascending as a tiebreaker.
func (c *Config) SortedNames() []string {
	names := make([]string, 0, len(c.Worktrees))
	for n := range c.Worktrees {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		mi, mj := c.Worktrees[names[i]], c.Worktrees[names[j]]
		if !mi.LastUsed.Equal(mj.LastUsed) {
			return mi.LastUsed.After(mj.LastUsed)
		}
		return mi.Slot < mj.Slot
	})
	return names
}
