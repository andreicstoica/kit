package liftoff

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setStateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("KIT_STATE_DIR", dir)
	return dir
}

func TestLoadState_Empty(t *testing.T) {
	setStateDir(t)
	s, err := LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if s.Schema != configSchema {
		t.Errorf("Schema = %d, want %d", s.Schema, configSchema)
	}
	if len(s.Worktrees) != 0 {
		t.Errorf("expected empty worktrees, got %d", len(s.Worktrees))
	}
}

func TestState_RoundTrip(t *testing.T) {
	dir := setStateDir(t)
	s, _ := LoadState()
	now := time.Now().UTC().Truncate(time.Second)
	s.Worktrees["voice-agent"] = WorktreeMeta{Slot: 1, Created: now, LastUsed: now}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.toml")); err != nil {
		t.Fatalf("config.toml not written: %v", err)
	}
	s2, err := LoadState()
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Worktrees["voice-agent"]
	if !ok {
		t.Fatal("worktree missing after reload")
	}
	if got.Slot != 1 {
		t.Errorf("Slot = %d, want 1", got.Slot)
	}
	if !got.Created.Equal(now) {
		t.Errorf("Created mismatch: %v vs %v", got.Created, now)
	}
}

func TestState_AllocateSlot_Sequential(t *testing.T) {
	setStateDir(t)
	s, _ := LoadState()
	a, err := s.AllocateSlot("a", nil)
	if err != nil || a != 1 {
		t.Errorf("first alloc = %d, %v; want 1, nil", a, err)
	}
	b, err := s.AllocateSlot("b", nil)
	if err != nil || b != 2 {
		t.Errorf("second alloc = %d, %v; want 2, nil", b, err)
	}
	c, err := s.AllocateSlot("c", nil)
	if err != nil || c != 3 {
		t.Errorf("third alloc = %d, %v; want 3, nil", c, err)
	}
}

func TestState_AllocateSlot_FillGap(t *testing.T) {
	setStateDir(t)
	s, _ := LoadState()
	_, _ = s.AllocateSlot("a", nil)
	_, _ = s.AllocateSlot("b", nil)
	_, _ = s.AllocateSlot("c", nil)
	s.FreeSlot("b")
	got, err := s.AllocateSlot("d", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Errorf("after freeing slot 2, expected reuse, got %d", got)
	}
}

func TestState_AllocateSlot_Idempotent(t *testing.T) {
	setStateDir(t)
	s, _ := LoadState()
	a, _ := s.AllocateSlot("voice-agent", nil)
	again, err := s.AllocateSlot("voice-agent", nil)
	if err != nil {
		t.Fatal(err)
	}
	if again != a {
		t.Errorf("re-alloc returned %d, want stable %d", again, a)
	}
}

func TestState_AllocateSlot_PortsFreeBumps(t *testing.T) {
	setStateDir(t)
	s, _ := LoadState()
	// Slot 1 reports busy; slot 2 free.
	got, err := s.AllocateSlot("a", func(slot int) bool { return slot >= 2 })
	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Errorf("expected bump to 2, got %d", got)
	}
}

func TestState_TouchLastUsed(t *testing.T) {
	setStateDir(t)
	s, _ := LoadState()
	_, _ = s.AllocateSlot("a", nil)
	old := s.Worktrees["a"].LastUsed
	time.Sleep(20 * time.Millisecond)
	s.TouchLastUsed("a")
	if !s.Worktrees["a"].LastUsed.After(old) {
		t.Errorf("LastUsed not advanced: %v vs %v", s.Worktrees["a"].LastUsed, old)
	}
}

func TestState_TouchLastUsed_Missing(t *testing.T) {
	setStateDir(t)
	s, _ := LoadState()
	s.TouchLastUsed("nope") // should not panic
}

func TestConfig_Schema2_NewFieldsRoundTrip(t *testing.T) {
	setStateDir(t)
	c, _ := LoadConfig()
	c.Worktrees["beta"] = WorktreeMeta{
		Slot:    3,
		Branch:  "acs/beta-cleanup",
		Path:    "/Users/acs/liftoff/beta",
		Adopted: true,
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	c2, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	m := c2.Worktrees["beta"]
	if m.Branch != "acs/beta-cleanup" || m.Path == "" || !m.Adopted {
		t.Fatalf("schema-2 fields not preserved: %+v", m)
	}
	if c2.Schema != 2 {
		t.Fatalf("schema not bumped to 2: %d", c2.Schema)
	}
}

func TestConfig_SettingsRoundTrip(t *testing.T) {
	setStateDir(t)
	c, _ := LoadConfig()
	c.Settings.Root = "/Users/acs/liftoff"
	c.Settings.Editor = "zed"
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	c2, _ := LoadConfig()
	if c2.Settings.Root != "/Users/acs/liftoff" || c2.Settings.Editor != "zed" {
		t.Fatalf("settings not preserved: %+v", c2.Settings)
	}
}

func TestState_SortedNames_RecencyFirst(t *testing.T) {
	setStateDir(t)
	s, _ := LoadState()
	now := time.Now().UTC()
	s.Worktrees["old"] = WorktreeMeta{Slot: 1, LastUsed: now.Add(-2 * time.Hour)}
	s.Worktrees["new"] = WorktreeMeta{Slot: 2, LastUsed: now}
	s.Worktrees["mid"] = WorktreeMeta{Slot: 3, LastUsed: now.Add(-1 * time.Hour)}
	got := s.SortedNames()
	want := []string{"new", "mid", "old"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, n := range want {
		if got[i] != n {
			t.Errorf("[%d] = %s, want %s", i, got[i], n)
		}
	}
}
