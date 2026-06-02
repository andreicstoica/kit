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
	useGH := HasGH()
	var out []MergedCandidate
	for _, w := range wts {
		if w.IsMaster(l) || w.Bare {
			continue
		}
		name := w.Name()
		// `git branch --merged` also lists branches that never diverged (tip is
		// an ancestor of main), not just landed work — including a pushed branch
		// sitting at the exact main tip with all its work still uncommitted.
		// Require main to be strictly ahead (it absorbed commits this branch
		// lacks) AND an upstream before trusting the local heuristic; real merges
		// still get caught by the gh PR check below.
		if merged[w.Branch] && mainAheadOf(l.Master, l.MainBranch, w.Branch) && branchPushed(l.Master, w.Branch) {
			out = append(out, MergedCandidate{
				Name: name, Path: w.Path, Branch: w.Branch,
				Reason: "merged to " + l.MainBranch,
			})
			continue
		}
		if useGH {
			if state := prState(l.Master, w.Branch); state == "MERGED" || state == "CLOSED" {
				out = append(out, MergedCandidate{
					Name: name, Path: w.Path, Branch: w.Branch,
					Reason: "PR " + state,
				})
			}
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

// branchPushed reports whether branch has an upstream (was pushed at least
// once), gating the local merged-branch heuristic.
func branchPushed(masterRepo, branch string) bool {
	_, err := Run(masterRepo, "git", "rev-parse", "--verify", "--quiet", branch+"@{upstream}")
	return err == nil
}

// prState returns "MERGED", "CLOSED", "OPEN", or "" via `gh pr view <branch>`.
func prState(masterRepo, branch string) string {
	cmd := exec.Command("gh", "pr", "view", branch, "--json", "state")
	cmd.Dir = masterRepo
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	var resp struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return ""
	}
	return resp.State
}
