package cmd

import (
	"testing"

	"github.com/andreicstoica/kit/internal/liftoff"
)

func TestResolveSlot(t *testing.T) {
	cfg := &liftoff.Config{
		Worktrees: map[string]liftoff.WorktreeMeta{
			"feat-a":    {Slot: 3},
			"feat-zero": {Slot: 0}, // adopted record but unallocated
		},
	}

	tests := []struct {
		name     string
		cfg      *liftoff.Config
		wt       string
		wantSlot int
		wantErr  bool
	}{
		// Master rides slot 0 — must never be rejected, even with nil cfg.
		{"master with cfg", cfg, "master", 0, false},
		{"master nil cfg", nil, "master", 0, false},
		// Feature worktree with a real slot resolves it.
		{"feature with slot", cfg, "feat-a", 3, false},
		// Feature worktree unknown or slot 0 → "play first" error.
		{"unknown feature", cfg, "feat-x", 0, true},
		{"feature slot zero", cfg, "feat-zero", 0, true},
		{"feature nil cfg", nil, "feat-a", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSlot(tt.cfg, tt.wt)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got slot %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantSlot {
				t.Errorf("slot = %d, want %d", got, tt.wantSlot)
			}
		})
	}
}
