package tui

import (
	"slices"

	"github.com/andreicstoica/kit/internal/liftoff"
)

type playPlanResult struct {
	plan          liftoff.PlayPlan
	needsAdopt    bool
	adoptBranch   string
	adoptPath     string
	celeryVictim  string
	celeryPID     int
	celeryBlocked bool
}

func buildPlayPlan(layout liftoff.Layout, name, path string, selected []liftoff.Service) (playPlanResult, error) {
	cfg, err := liftoff.LoadConfig()
	if err != nil {
		return playPlanResult{}, err
	}
	slot, slotErr := cfg.SlotFor(name)
	if slotErr != nil {
		branch, wtPath := findBranchAndPath(layout, name)
		return playPlanResult{needsAdopt: true, adoptBranch: branch, adoptPath: wtPath}, nil
	}
	_ = liftoff.WithConfigLock(func(c *liftoff.Config) error {
		c.TouchLastUsed(name)
		return nil
	})
	plan := liftoff.PlayPlan{
		Worktree:     name,
		WorktreePath: path,
		Slot:         slot,
		Ports:        liftoff.PortsForSlot(slot),
		Services:     selected,
	}
	if (slices.Contains(selected, liftoff.SvcCelery) || slices.Contains(selected, liftoff.SvcBeat)) && liftoff.WorkerSharesBroker(path) {
		if owner, pid := liftoff.FindSharedCeleryOwner(name); owner != "" {
			return playPlanResult{plan: plan, celeryBlocked: true, celeryVictim: owner, celeryPID: pid}, nil
		}
	}
	return playPlanResult{plan: plan}, nil
}

func findBranchAndPath(layout liftoff.Layout, name string) (string, string) {
	wts, err := layout.ListWorktrees()
	if err != nil {
		return "", ""
	}
	for _, w := range wts {
		if w.IsMaster(layout) || w.Bare {
			continue
		}
		if w.Name() == name {
			return w.Branch, w.Path
		}
	}
	return "", ""
}
