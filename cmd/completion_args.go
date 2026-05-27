package cmd

import (
	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/spf13/cobra"
)

// completeWorktreeNames returns the list of worktree names eligible for
// the first positional arg of commands like swap / play / pause / restart
// / log / wash / links / diff. Includes "master" plus every managed
// worktree from config, with on-disk feature worktrees as a fallback so
// completion still works before adoption.
func completeWorktreeNames(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) >= 1 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	names := []string{"master"}
	seen := map[string]bool{"master": true}

	if cfg, err := liftoff.LoadConfig(); err == nil && cfg != nil {
		for n := range cfg.Worktrees {
			if seen[n] {
				continue
			}
			seen[n] = true
			names = append(names, n)
		}
	}
	// Fall back to on-disk worktrees so completion suggests pre-adoption ones.
	layout := liftoff.DefaultLayout()
	if wts, err := layout.ListWorktrees(); err == nil {
		for _, w := range wts {
			if w.Bare {
				continue
			}
			n := w.Name()
			if w.IsMaster(layout) {
				n = "master"
			}
			if seen[n] {
				continue
			}
			seen[n] = true
			names = append(names, n)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeAdoptCandidates is like completeWorktreeNames but limits the
// results to worktrees that aren't already in config — i.e. valid adopt
// targets.
func completeAdoptCandidates(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) >= 1 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cfg, err := liftoff.LoadConfig()
	if err != nil || cfg == nil {
		cfg = &liftoff.Config{Worktrees: map[string]liftoff.WorktreeMeta{}}
	}
	layout := liftoff.DefaultLayout()
	cands, err := layout.FindAdoptCandidates(cfg)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	names := make([]string, 0, len(cands))
	for _, c := range cands {
		names = append(names, c.Name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
