package liftoff

import (
	"os/exec"
	"strings"
)

// MergedCandidate is one worktree eligible for cleanup.
type MergedCandidate struct {
	Name   string
	Path   string
	Branch string
	Head   string
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

	var branches []string
	for _, w := range wts {
		if !w.IsMaster(l) && !w.Bare && !w.Detached && !w.Missing && !w.Locked && w.Branch != l.MainBranch {
			branches = append(branches, w.Branch)
		}
	}
	prStates := map[string]PRStatus{}
	if HasGH() {
		prStates, err = l.RemotePRStatuses(branches)
		if err != nil {
			return nil, err
		}
	}

	var out []MergedCandidate
	for _, w := range wts {
		if w.IsMaster(l) || w.Bare || w.Detached || w.Missing || w.Locked || w.Branch == l.MainBranch {
			continue
		}
		pr := prStates[w.Branch]
		if pr.State == "OPEN" {
			continue
		}
		name := w.Name()
		if merged[w.Branch] && mainAheadOf(l.Master, l.MainBranch, w.Branch) && branchHasOwnUpstream(l.Master, w.Branch) {
			out = append(out, MergedCandidate{
				Name: name, Path: w.Path, Branch: w.Branch, Head: w.Head,
				Reason: "merged to " + l.MainBranch,
				Dirty:  IsDirty(w.Path),
			})
			continue
		}
		if (pr.State == "MERGED" || pr.State == "CLOSED") && pr.HeadOID != "" && pr.HeadOID == w.Head {
			out = append(out, MergedCandidate{
				Name: name, Path: w.Path, Branch: w.Branch, Head: w.Head,
				Reason: "PR " + pr.State,
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
