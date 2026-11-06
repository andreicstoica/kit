package liftoff

import "testing"

func TestParentOfStack(t *testing.T) {
	stack := []StackEntry{
		{Branch: "master"},
		{Branch: "feat-a"},
		{Branch: "feat-b", Current: true},
	}
	if got := ParentOfStack(stack); got != "feat-a" {
		t.Errorf("ParentOfStack = %q, want feat-a", got)
	}
	if got := ParentOfStack([]StackEntry{{Branch: "master", Current: true}}); got != "" {
		t.Errorf("trunk parent = %q, want empty", got)
	}
	if got := ParentOfStack(nil); got != "" {
		t.Errorf("nil parent = %q, want empty", got)
	}
}

func TestSlotFor(t *testing.T) {
	cfg := &Config{Worktrees: map[string]WorktreeMeta{"feat-a": {Slot: 3}}}
	if slot, err := cfg.SlotFor("master"); err != nil || slot != 0 {
		t.Errorf("master = %d, %v; want 0, nil", slot, err)
	}
	if slot, err := cfg.SlotFor("feat-a"); err != nil || slot != 3 {
		t.Errorf("feat-a = %d, %v; want 3, nil", slot, err)
	}
	if _, err := cfg.SlotFor("unknown"); err == nil {
		t.Error("unknown: want error, got nil")
	}
	var nilCfg *Config
	if _, err := nilCfg.SlotFor("feat-a"); err == nil {
		t.Error("nil config: want error, got nil")
	}
	if slot, err := nilCfg.SlotFor("master"); err != nil || slot != 0 {
		t.Errorf("nil master = %d, %v; want 0, nil", slot, err)
	}
}
