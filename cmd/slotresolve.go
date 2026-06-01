package cmd

import (
	"fmt"

	"github.com/andreicstoica/kit/internal/liftoff"
)

// resolveSlot returns the port slot for a worktree, enforcing the
// "must be played first" guard for feature worktrees.
//
// Master is special: it rides slot 0 by convention (master defaults —
// see liftoff.PortsForSlot), never gets a slot allocated, and never
// needs adoption. The `Slot == 0` guard means "unallocated" only for
// feature worktrees, so master is exempt — otherwise commands like
// `kit restart` wrongly reject it with "has no slot".
func resolveSlot(cfg *liftoff.Config, name string) (int, error) {
	if name == "master" {
		return 0, nil
	}
	if cfg == nil {
		return 0, fmt.Errorf("%s has no slot — run `kit play %s` first", name, name)
	}
	meta, ok := cfg.Worktrees[name]
	if !ok || meta.Slot == 0 {
		return 0, fmt.Errorf("%s has no slot — run `kit play %s` first", name, name)
	}
	return meta.Slot, nil
}
