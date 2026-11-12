package liftoff

import (
	"os/exec"
	"strconv"
	"strings"
)

// MergedKind is why a worktree is eligible for cleanup. It drives the safety
// rule; Reason carries the wording shown to the user.
type MergedKind int

const (
	KindUnknown MergedKind = iota
	KindMergedIntoTrunk
	KindMergedPR
	KindClosedPR
)

// MergedCandidate is one worktree eligible for cleanup.
type MergedCandidate struct {
	Name   string
	Path   string
	Branch string
	Head   string
	Kind   MergedKind
	Reason string // display text; never parsed
	Dirty  bool   // uncommitted/untracked changes in the worktree — wash destroys them
	// LocalWork means the checkout holds changes that are not in trunk.
	LocalWork bool
	// Unverified means the checkout's content could not be compared with
	// trunk, so local work may or may not be included.
	Unverified bool
}

// Blockers lists every reason washing this candidate could lose work. It is
// the single owner of the safety rule; Safe is exactly "no blockers".
func (c MergedCandidate) Blockers() []string {
	var reasons []string
	switch c.Kind {
	case KindMergedIntoTrunk, KindMergedPR:
	case KindClosedPR:
		reasons = append(reasons, "closed without merge")
	default:
		reasons = append(reasons, "not a merged workspace")
	}
	if c.Dirty {
		reasons = append(reasons, "dirty — uncommitted changes")
	}
	if c.LocalWork {
		reasons = append(reasons, "local work not in trunk")
	}
	if c.Unverified {
		reasons = append(reasons, "local changes could not be verified")
	}
	return reasons
}

// Safe reports whether washing the checkout loses nothing.
func (c MergedCandidate) Safe() bool {
	return len(c.Blockers()) == 0
}

// MergedScan is the result of one merged-worktree sweep. Warnings describe
// reduced coverage — missing GitHub tooling or a partial lookup failure —
// without discarding candidates that local Git evidence could prove.
type MergedScan struct {
	Candidates []MergedCandidate
	Warnings   []string
}

// HasGH returns true if the GitHub CLI is on PATH.
func HasGH() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// ScanMergedWorktrees returns worktrees whose branch is merged into the trunk
// or whose PR is merged/closed. Skips master itself and bare entries.
func (l Layout) ScanMergedWorktrees() (MergedScan, error) {
	wts, err := l.ListWorktrees()
	if err != nil {
		return MergedScan{}, err
	}
	refs := trunkRefs(l.Master, l.MainBranch)
	merged := mergedBranchSet(l.Master, l.MainBranch, refs)

	var eligible []Worktree
	for _, w := range wts {
		if !w.IsMaster(l) && !w.Bare && !w.Detached && !w.Missing && !w.Locked && w.Branch != l.MainBranch {
			eligible = append(eligible, w)
		}
	}

	var scan MergedScan
	prStates := map[string]PRStatus{}
	if !HasGH() {
		scan.Warnings = append(scan.Warnings,
			"gh is not installed, so merged PRs were not checked; only branches merged into "+l.MainBranch+" were found")
	} else {
		branches := make([]string, 0, len(eligible))
		for _, w := range eligible {
			branches = append(branches, w.Branch)
		}
		statuses, lookupErr := l.RemotePRStatuses(branches)
		prStates = statuses
		if lookupErr != nil {
			scan.Warnings = append(scan.Warnings,
				"some GitHub PR lookups failed, so PR-based results are incomplete: "+lookupErr.Error())
		}
	}

	needsReview := false
	for _, w := range eligible {
		pr := prStates[w.Branch]
		if pr.State == "OPEN" {
			continue
		}
		candidate := MergedCandidate{
			Name:   w.Name(),
			Path:   w.Path,
			Branch: w.Branch,
			Head:   w.Head,
		}
		if merged[w.Branch] && trunkAheadOf(l.Master, refs, w.Branch) && branchHasOwnUpstream(l.Master, w.Branch) {
			candidate.Kind = KindMergedIntoTrunk
			candidate.Reason = "merged to " + l.MainBranch
			candidate.Dirty = IsDirty(w.Path)
			scan.Candidates = append(scan.Candidates, candidate)
			continue
		}
		switch pr.State {
		case "MERGED":
			candidate.Kind = KindMergedPR
			contained, verified := l.mergedContentInTrunk(w.Head, refs)
			switch {
			case contained:
				candidate.Reason = "PR MERGED"
			case !verified:
				candidate.Reason = "PR MERGED — could not verify local changes"
				candidate.Unverified = true
				needsReview = true
			default:
				candidate.Reason = "PR MERGED — local work not in " + l.MainBranch
				candidate.LocalWork = true
			}
			candidate.Dirty = IsDirty(w.Path)
			scan.Candidates = append(scan.Candidates, candidate)
		case "CLOSED":
			if pr.HeadOID != "" && pr.HeadOID == w.Head {
				candidate.Kind = KindClosedPR
				candidate.Reason = "PR CLOSED"
				candidate.Dirty = IsDirty(w.Path)
				scan.Candidates = append(scan.Candidates, candidate)
			}
		}
	}
	if needsReview {
		scan.Warnings = append(scan.Warnings,
			"some merged PRs could not be compared with trunk; they are listed but not selected by default (git merge-tree --write-tree requires git 2.38+)")
	}
	return scan, nil
}

// mergedContentInTrunk reports whether every change in the checkout is
// already in trunk. Merging the head into a trunk ref is a no-op exactly when
// the work landed, which covers squash, rebase, merge-queue, and restacked
// branches without trusting the PR head (it can move after the merge).
func (l Layout) mergedContentInTrunk(head string, refs []string) (contained, verified bool) {
	blocked, differs := false, false
	for _, ref := range refs {
		trunkTree, err := Run(l.Master, "git", "rev-parse", ref+"^{tree}")
		if err != nil {
			continue
		}
		out, err := Run(l.Master, "git", "merge-tree", "--write-tree", ref, head)
		if err != nil {
			continue
		}
		tree := firstGitOID(out)
		if tree == "" {
			continue
		}
		if tree != strings.TrimSpace(trunkTree) {
			differs = true
			continue
		}
		if l.customMergeDriverApplies(head, ref) {
			blocked = true
			continue
		}
		return true, true
	}
	// A ref can be stale or driver-blocked, so a difference in one ref only
	// proves unmerged work when no ref was blocked.
	return false, differs && !blocked
}

// customMergeDriverApplies reports whether a .gitattributes merge driver could
// hide checkout content during a merge. Such a driver can make the merged tree
// equal trunk while silently discarding one side, which would turn "no diff"
// into false proof of containment.
func (l Layout) customMergeDriverApplies(head, ref string) bool {
	out, err := Run(l.Master, "git", "diff", "--name-only", head, ref)
	if err != nil {
		return true
	}
	var paths []string
	for _, path := range strings.Split(out, "\n") {
		if path != "" {
			paths = append(paths, path)
		}
	}
	if len(paths) == 0 {
		return false
	}
	args := append([]string{"-C", l.Master, "check-attr", "merge", "--"}, paths...)
	attrs, err := exec.Command("git", args...).Output()
	if err != nil {
		return true
	}
	for _, line := range strings.Split(strings.TrimSpace(string(attrs)), "\n") {
		if !strings.HasSuffix(line, ": merge: unspecified") {
			return true
		}
	}
	return false
}

// firstGitOID returns the first object name in output. merge-tree can print
// warnings before the tree OID.
func firstGitOID(out string) string {
	for _, token := range strings.Fields(out) {
		if len(token) != 40 && len(token) != 64 {
			continue
		}
		if _, err := strconv.ParseUint(token[:16], 16, 64); err == nil {
			return token
		}
	}
	return ""
}

// trunkRefs returns the refs that count as landed trunk. A fetched
// origin/<main> catches merges that landed upstream while the local branch is
// stale; the local branch is still included for local-only merges.
func trunkRefs(masterRepo, mainBranch string) []string {
	refs := []string{mainBranch}
	remote := "origin/" + mainBranch
	if _, err := exec.Command("git", "-C", masterRepo, "rev-parse", "--verify", "--quiet", remote).Output(); err == nil {
		refs = append(refs, remote)
	}
	return refs
}

func mergedBranchSet(masterRepo, mainBranch string, refs []string) map[string]bool {
	m := map[string]bool{}
	for _, ref := range refs {
		out, err := exec.Command("git", "-C", masterRepo, "branch", "--merged", ref, "--format=%(refname:short)").Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			b := strings.TrimSpace(line)
			if b == "" || b == mainBranch || b == "origin/"+mainBranch {
				continue
			}
			m[b] = true
		}
	}
	return m
}

// trunkAheadOf reports whether trunk has moved past the branch tip. False when
// the branch tip equals a trunk tip — a branch sitting exactly at trunk never
// had work land, so it isn't merged. False also when no trunk ref could be
// read, so an unreadable repo never marks work for deletion.
func trunkAheadOf(masterRepo string, refs []string, branch string) bool {
	branchTip, err := Run(masterRepo, "git", "rev-parse", "--verify", branch)
	if err != nil {
		return false
	}
	branchTip = strings.TrimSpace(branchTip)
	checked := false
	for _, ref := range refs {
		tip, err := Run(masterRepo, "git", "rev-parse", "--verify", ref)
		if err != nil {
			continue
		}
		checked = true
		if strings.TrimSpace(tip) == branchTip {
			return false
		}
	}
	return checked
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
