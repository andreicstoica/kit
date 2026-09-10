package liftoff

import (
	"encoding/json"
	"os/exec"
	"strings"
)

// MergedCandidate is one worktree eligible for cleanup.
type MergedCandidate struct {
	Name   string
	Path   string
	Branch string
	Reason string // "merged to master" | "PR MERGED" | "PR CLOSED"
	Dirty  bool   // uncommitted/untracked changes in the worktree — wash destroys them
}

// HasGH returns true if the GitHub CLI is on PATH.
func HasGH() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// FindMergedWorktrees returns worktrees whose branch is merged into the main
// branch or whose PR is merged/closed. Skips master itself and bare entries.
func (l Layout) FindMergedWorktrees() ([]MergedCandidate, error) {
	wts, err := l.ListWorktrees()
	if err != nil {
		return nil, err
	}
	merged := mergedBranches(l.Master, l.MainBranch)

	// Collect non-master branches that need a PR state check.
	var needGH []int // indices into wts
	for i, w := range wts {
		if w.IsMaster(l) || w.Bare {
			continue
		}
		if merged[w.Branch] && mainAheadOf(l.Master, l.MainBranch, w.Branch) && branchHasOwnUpstream(l.Master, w.Branch) {
			continue // already known merged via local git
		}
		needGH = append(needGH, i)
	}

	// Batch-query PR states in one gh call instead of one per branch.
	prStates := map[string]string{} // branch → state
	if HasGH() && len(needGH) > 0 {
		prStates = batchPRStates(l.Master, wts, needGH)
	}

	var out []MergedCandidate
	for _, w := range wts {
		if w.IsMaster(l) || w.Bare {
			continue
		}
		name := w.Name()
		if merged[w.Branch] && mainAheadOf(l.Master, l.MainBranch, w.Branch) && branchHasOwnUpstream(l.Master, w.Branch) {
			out = append(out, MergedCandidate{
				Name: name, Path: w.Path, Branch: w.Branch,
				Reason: "merged to " + l.MainBranch,
				Dirty:  IsDirty(w.Path),
			})
			continue
		}
		if state := prStates[w.Branch]; state == "MERGED" || state == "CLOSED" {
			out = append(out, MergedCandidate{
				Name: name, Path: w.Path, Branch: w.Branch,
				Reason: "PR " + state,
				Dirty:  IsDirty(w.Path),
			})
		}
	}
	return out, nil
}

// mergedBranches returns the set of branch names already merged into mainBranch.
func mergedBranches(masterRepo, mainBranch string) map[string]bool {
	out, err := exec.Command("git", "-C", masterRepo, "branch", "--merged", mainBranch, "--format=%(refname:short)").Output()
	if err != nil {
		return map[string]bool{}
	}
	m := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		b := strings.TrimSpace(line)
		if b == "" || b == mainBranch {
			continue
		}
		m[b] = true
	}
	return m
}

// mainAheadOf reports whether mainBranch has at least one commit that branch
// lacks (mainBranch is strictly ahead). False when their tips are identical —
// a branch sitting exactly at main never had work land, so it isn't merged.
func mainAheadOf(masterRepo, mainBranch, branch string) bool {
	out, err := Run(masterRepo, "git", "rev-list", "--count", branch+".."+mainBranch)
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) != "0"
}

// branchHasOwnUpstream reports whether branch tracks its own remote
// counterpart (…/<branch>), i.e. it was pushed at least once. A borrowed
// upstream like origin/master (how `git worktree add` off master often leaves
// a branch before its first push) must not count: it made a never-pushed
// branch parked at an old master tip look "merged" while all of its work was
// still uncommitted in the tree.
func branchHasOwnUpstream(masterRepo, branch string) bool {
	out, err := Run(masterRepo, "git", "rev-parse", "--abbrev-ref", "--verify", "--quiet", branch+"@{upstream}")
	if err != nil {
		return false
	}
	return strings.HasSuffix(strings.TrimSpace(out), "/"+branch)
}

// batchPRStates queries gh for all PR states in one call, returning a map of
// branch→state for branches that have an open, merged, or closed PR.
func batchPRStates(masterRepo string, wts []Worktree, indices []int) map[string]string {
	if len(indices) == 0 {
		return nil
	}
	// Use a single gh call: list all PRs, filter client-side.
	cmd := exec.Command("gh", "pr", "list", "--state", "all", "--json", "headRefName,state", "--limit", "200")
	cmd.Dir = masterRepo
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	type prEntry struct {
		HeadRefName string `json:"headRefName"`
		State       string `json:"state"`
	}
	var all []prEntry
	if err := json.Unmarshal(out, &all); err != nil {
		return nil
	}
	wanted := make(map[string]bool, len(indices))
	for _, i := range indices {
		wanted[wts[i].Branch] = true
	}
	result := make(map[string]string)
	for _, pr := range all {
		if wanted[pr.HeadRefName] {
			result[pr.HeadRefName] = pr.State
		}
	}
	return result
}
