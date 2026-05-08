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
	stateSchema   = 1
	stateFileName = "state.toml"
	maxSlot       = 99
)

// WorktreeMeta is the persisted record for one worktree in state.toml.
type WorktreeMeta struct {
	Slot     int       `toml:"slot"`
	Created  time.Time `toml:"created"`
	LastUsed time.Time `toml:"last_used"`
}

// State is the on-disk shape of state.toml.
type State struct {
	Schema    int                     `toml:"schema"`
	Worktrees map[string]WorktreeMeta `toml:"worktrees,omitempty"`
}

// StatePath returns ~/.config/kit/state.toml (or $KIT_STATE_DIR/state.toml).
func StatePath() string {
	if v := os.Getenv("KIT_STATE_DIR"); v != "" {
		return filepath.Join(v, stateFileName)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "kit", stateFileName)
}

// LoadState reads state.toml; returns an empty State if the file is absent.
func LoadState() (*State, error) {
	path := StatePath()
	s := &State{Schema: stateSchema, Worktrees: map[string]WorktreeMeta{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, err
	}
	if _, err := toml.Decode(string(data), s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if s.Worktrees == nil {
		s.Worktrees = map[string]WorktreeMeta{}
	}
	if s.Schema == 0 {
		s.Schema = stateSchema
	}
	return s, nil
}

// Save writes state.toml atomically.
func (s *State) Save() error {
	path := StatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	enc := toml.NewEncoder(f)
	if err := enc.Encode(s); err != nil {
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
func (s *State) AllocateSlot(name string, portsFree func(slot int) bool) (int, error) {
	if existing, ok := s.Worktrees[name]; ok && existing.Slot > 0 {
		return existing.Slot, nil
	}
	used := map[int]bool{0: true}
	for _, m := range s.Worktrees {
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
		s.Worktrees[name] = WorktreeMeta{
			Slot:     slot,
			Created:  now,
			LastUsed: now,
		}
		return slot, nil
	}
	return 0, fmt.Errorf("no free slot ≤ %d (you have a lot of worktrees!)", maxSlot)
}

// FreeSlot removes a worktree's record. Idempotent.
func (s *State) FreeSlot(name string) {
	delete(s.Worktrees, name)
}

// TouchLastUsed updates the LastUsed timestamp for a worktree.
// No-op if the worktree is absent.
func (s *State) TouchLastUsed(name string) {
	m, ok := s.Worktrees[name]
	if !ok {
		return
	}
	m.LastUsed = time.Now().UTC()
	s.Worktrees[name] = m
}

// SortedNames returns worktree names sorted by LastUsed descending,
// then by Slot ascending as a tiebreaker.
func (s *State) SortedNames() []string {
	names := make([]string, 0, len(s.Worktrees))
	for n := range s.Worktrees {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		mi, mj := s.Worktrees[names[i]], s.Worktrees[names[j]]
		if !mi.LastUsed.Equal(mj.LastUsed) {
			return mi.LastUsed.After(mj.LastUsed)
		}
		return mi.Slot < mj.Slot
	})
	return names
}
