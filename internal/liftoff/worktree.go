package liftoff

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Worktree describes one entry in `git worktree list --porcelain` output.
type Worktree struct {
	Path     string // absolute path on disk
	Branch   string // branch name (without refs/heads/ prefix)
	Head     string // commit SHA at HEAD
	Detached bool
	Bare     bool
	Locked   bool
	Missing  bool
}

// Name returns the canonical kit name for this worktree.
// Strips a leading "liftoff-" from the path basename if present.
func (w Worktree) Name() string {
	base := filepath.Base(w.Path)
	return strings.TrimPrefix(base, "liftoff-")
}

// IsMaster returns true if this is the master worktree itself.
func (w Worktree) IsMaster(l Layout) bool {
	return pathKey(w.Path) == pathKey(l.Master)
}

// HasLegacyPrefix returns true if the worktree dir is named liftoff-<x>.
func (w Worktree) HasLegacyPrefix() bool {
	base := filepath.Base(w.Path)
	return strings.HasPrefix(base, "liftoff-") && base != "liftoff-app-master"
}

// parseWorktrees parses `git worktree list --porcelain` output. Records are
// separated by blank lines and start with a "worktree <path>" line.
func parseWorktrees(out string) []Worktree {
	var entries []Worktree
	var cur Worktree
	flush := func() {
		if cur.Path != "" {
			entries = append(entries, cur)
		}
		cur = Worktree{}
	}
	s := bufio.NewScanner(strings.NewReader(out))
	for s.Scan() {
		line := s.Text()
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			cur.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "detached":
			cur.Detached = true
		case line == "bare":
			cur.Bare = true
		case line == "locked" || strings.HasPrefix(line, "locked "):
			cur.Locked = true
		}
	}
	flush()
	return entries
}

// ListWorktrees reads git's worktree list from the master repo and marks
// checkouts whose directory is absent.
func (l Layout) ListWorktrees() ([]Worktree, error) {
	cmd := exec.Command("git", "-C", l.Master, "worktree", "list", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	wts := parseWorktrees(string(out))
	for i := range wts {
		_, err := os.Stat(wts[i].Path)
		wts[i].Missing = errors.Is(err, os.ErrNotExist)
	}
	return wts, nil
}

// WorktreeSet indexes git worktrees by name, branch, and path so config
// records resolve with one matching rule instead of per-caller maps.
type WorktreeSet struct {
	entries  []Worktree
	byName   map[string]int
	byBranch map[string]int
	byPath   map[string]int
}

func NewWorktreeSet(wts []Worktree) WorktreeSet {
	s := WorktreeSet{
		entries:  wts,
		byName:   make(map[string]int, len(wts)),
		byBranch: make(map[string]int, len(wts)),
		byPath:   make(map[string]int, len(wts)),
	}
	for i, wt := range wts {
		s.byName[wt.Name()] = i
		if wt.Branch != "" {
			s.byBranch[wt.Branch] = i
		}
		s.byPath[pathKey(wt.Path)] = i
	}
	return s
}

// Find resolves a config record against the live worktrees, preferring a
// matching name, then branch, then checkout path.
func (s WorktreeSet) Find(name, branch, path string) (Worktree, bool) {
	if name != "" {
		if i, ok := s.byName[name]; ok {
			return s.entries[i], true
		}
	}
	if branch != "" {
		if i, ok := s.byBranch[branch]; ok {
			return s.entries[i], true
		}
	}
	if path != "" {
		if i, ok := s.byPath[pathKey(path)]; ok {
			return s.entries[i], true
		}
	}
	return Worktree{}, false
}

func (s WorktreeSet) At(path string) (Worktree, bool) {
	i, ok := s.byPath[pathKey(path)]
	if !ok {
		return Worktree{}, false
	}
	return s.entries[i], true
}

// IsDirty returns true if the worktree has uncommitted changes. A failed git
// call counts as dirty so cleanup never treats an unknown checkout as clean.
func IsDirty(worktreePath string) bool {
	out, err := exec.Command("git", "-C", worktreePath, "status", "--porcelain").Output()
	if err != nil {
		return true
	}
	return len(strings.TrimSpace(string(out))) > 0
}

// AheadBehind returns (ahead, behind) commits vs origin/<main>. Returns
// (0, 0) if it can't determine.
func (l Layout) AheadBehind(worktreePath string) (ahead, behind int) {
	out, err := exec.Command("git", "-C", worktreePath, "rev-list", "--left-right", "--count",
		"HEAD..."+"origin/"+l.MainBranch).Output()
	if err != nil {
		return 0, 0
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) != 2 {
		return 0, 0
	}
	return atoi(parts[0]), atoi(parts[1])
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// cleanPath returns an absolute, cleaned form. Imported pane CWDs are stored
// with it, so it does not resolve symlinks.
func cleanPath(path string) string {
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

// pathKey normalizes a path for identity comparisons. Git reports resolved
// paths while layout paths may not be, so existing paths are symlink-resolved;
// deleted paths keep their absolute form.
func pathKey(path string) string {
	cleaned := cleanPath(path)
	if cleaned == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		return resolved
	}
	return cleaned
}

// FetchMain runs `git fetch origin` from the master repo. Avoids the
// refspec form (`<main>:<main>`), which fails with exit 128 when the
// local main branch is checked out anywhere (master worktree on a
// feature branch, etc.). We branch off `origin/<main>` below, so the
// local ref doesn't need to be touched.
func (l Layout) FetchMain(onLine LineFn) error {
	args := []string{"-C", l.Master, "fetch", "origin", l.MainBranch}
	return RunStream("", "git", args, onLine)
}

// FastForwardTrunk advances the master checkout to origin/<main> when it is on
// the trunk branch with no local changes. It reports whether trunk actually
// moved.
func (l Layout) FastForwardTrunk(onLine LineFn) (bool, error) {
	branch, err := l.MasterBranch()
	if err != nil || branch != l.MainBranch || IsDirty(l.Master) {
		return false, nil
	}
	before, err := l.MasterHead()
	if err != nil {
		return false, err
	}
	args := []string{"-C", l.Master, "merge", "--ff-only", "origin/" + l.MainBranch}
	if err := RunStream("", "git", args, onLine); err != nil {
		return true, err
	}
	after, err := l.MasterHead()
	if err != nil {
		return true, err
	}
	return before != after, nil
}

// AddWorktree creates a new worktree at path with a new branch off
// origin/<main> (so it picks up the latest upstream commit even when
// the local main ref hasn't been fast-forwarded).
func (l Layout) AddWorktree(name, path string, onLine LineFn) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("worktree path already exists: %s", path)
	}
	args := []string{"-C", l.Master, "worktree", "add", path, "-b", name, "origin/" + l.MainBranch}
	return RunStream("", "git", args, onLine)
}

// RemoveWorktree removes the worktree at path (force).
func (l Layout) RemoveWorktree(path string, onLine LineFn) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		wts, err := l.ListWorktrees()
		if err != nil {
			return err
		}
		if _, registered := NewWorktreeSet(wts).At(path); !registered {
			return nil
		}
	} else if err != nil {
		return err
	}
	args := []string{"-C", l.Master, "worktree", "remove", path, "--force"}
	return RunStream("", "git", args, onLine)
}

// DeleteBranch force-deletes the local branch in the master repo.
// Non-fatal: returns nil if branch is gone.
func (l Layout) DeleteBranch(branch string, onLine LineFn) error {
	cmd := exec.Command("git", "-C", l.Master, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return nil
		}
		return fmt.Errorf("check branch %q: %w", branch, err)
	}
	args := []string{"-C", l.Master, "branch", "-D", branch}
	return RunStream("", "git", args, onLine)
}

// MasterIsRepo returns true if the master path exists and looks like a git repo.
func (l Layout) MasterIsRepo() bool {
	if _, err := os.Stat(l.Master); err != nil {
		return false
	}
	if _, err := os.Stat(l.Master + "/.git"); err != nil {
		return false
	}
	return true
}

// VerifyGitCheckout checks both the managing repository's registration and the
// checkout's current Git identity; a stale registration alone is insufficient.
func (l Layout) VerifyGitCheckout(name, path, branch string) error {
	worktrees, err := l.ListWorktrees()
	if err != nil {
		return fmt.Errorf("check Git worktree registration: %w", err)
	}
	wt, ok := NewWorktreeSet(worktrees).At(path)
	if !ok || wt.IsMaster(l) != (name == "master") || wt.Missing || wt.Bare ||
		(branch != "" && wt.Branch != branch) {
		return fmt.Errorf("%s is not the registered Git checkout for %q", path, name)
	}
	masterCommon, err := Run(l.Master, "git", "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return fmt.Errorf("resolve master Git directory: %w", err)
	}
	checkoutCommon, err := Run(path, "git", "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil || pathKey(checkoutCommon) != pathKey(masterCommon) {
		return fmt.Errorf("%s is not a checkout of the managing repository", path)
	}
	gitTop, err := Run(path, "git", "rev-parse", "--show-toplevel")
	if err != nil || pathKey(gitTop) != pathKey(path) {
		return fmt.Errorf("%s is not the root of the registered checkout", path)
	}
	return nil
}
