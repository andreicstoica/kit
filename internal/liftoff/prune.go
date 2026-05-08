package liftoff

import (
	"encoding/json"
	"os/exec"
	"strings"
)

// PruneCandidate is one worktree eligible for cleanup.
type PruneCandidate struct {
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
func (l Layout) FindMergedWorktrees() ([]PruneCandidate, error) {
	wts, err := l.ListWorktrees()
	if err != nil {
		return nil, err
	}
	merged := mergedBranches(l.Master, l.MainBranch)
	useGH := HasGH()
	var out []PruneCandidate
	for _, w := range wts {
		if w.IsMaster(l) || w.Bare {
			continue
		}
		name := w.Name()
		if merged[w.Branch] {
			out = append(out, PruneCandidate{
				Name: name, Path: w.Path, Branch: w.Branch,
				Reason: "merged to " + l.MainBranch,
			})
			continue
		}
		if useGH {
			if state := prState(l.Master, w.Branch); state == "MERGED" || state == "CLOSED" {
				out = append(out, PruneCandidate{
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
