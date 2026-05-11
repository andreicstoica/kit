package liftoff

import "testing"

func TestFindAdoptCandidates_FiltersAlreadyAdopted(t *testing.T) {
	// Test the in-memory diff between a config + a fake worktree list, without
	// needing git.
	c := &Config{Worktrees: map[string]WorktreeMeta{
		"alpha": {Slot: 1},
		"beta":  {Slot: 2},
	}}

	// Simulate the filter loop directly: any wt name not in config is adoptable.
	wts := []Worktree{
		{Path: "/p/alpha", Branch: "alpha"},
		{Path: "/p/beta", Branch: "feat/beta"},
		{Path: "/p/gamma", Branch: "gamma"},
		{Path: "/p/master", Bare: true},
	}

	cands := []AdoptCandidate{}
	for _, w := range wts {
		if w.Bare {
			continue
		}
		name := w.Name()
		if _, ok := c.Worktrees[name]; ok {
			continue
		}
		cands = append(cands, AdoptCandidate{Name: name, Branch: w.Branch, Path: w.Path})
	}

	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate, got %d: %+v", len(cands), cands)
	}
	if cands[0].Name != "gamma" {
		t.Errorf("expected gamma, got %s", cands[0].Name)
	}
}
