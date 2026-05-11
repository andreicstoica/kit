package liftoff

import (
	"bufio"
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
}

// Name returns the canonical kit name for this worktree.
// Strips a leading "liftoff-" from the path basename if present.
func (w Worktree) Name() string {
	base := filepath.Base(w.Path)
	return strings.TrimPrefix(base, "liftoff-")
}

// IsMaster returns true if this is the master worktree itself.
func (w Worktree) IsMaster(l Layout) bool {
	return strings.TrimRight(w.Path, "/") == strings.TrimRight(l.Master, "/")
}

// HasLegacyPrefix returns true if the worktree dir is named liftoff-<x>.
func (w Worktree) HasLegacyPrefix() bool {
	base := filepath.Base(w.Path)
	return strings.HasPrefix(base, "liftoff-") && base != "liftoff-app-master"
}

// ListWorktrees parses `git worktree list --porcelain` from the master repo.
func (l Layout) ListWorktrees() ([]Worktree, error) {
	cmd := exec.Command("git", "-C", l.Master, "worktree", "list", "--porcelain")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	var out []Worktree
	var cur Worktree
	flush := func() {
		if cur.Path != "" {
			out = append(out, cur)
		}
		cur = Worktree{}
	}
	s := bufio.NewScanner(stdout)
	for s.Scan() {
		line := s.Text()
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "worktree "):
			cur.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			cur.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "detached":
			cur.Detached = true
		case line == "bare":
			cur.Bare = true
		}
	}
	flush()
	if err := cmd.Wait(); err != nil {
		return nil, err
	}
	return out, nil
}

// IsDirty returns true if the worktree has uncommitted changes.
func IsDirty(worktreePath string) bool {
	cmd := exec.Command("git", "-C", worktreePath, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}

// AheadBehind returns (ahead, behind) commits vs origin/<main>.
// Returns (0, 0) if it can't determine.
func (l Layout) AheadBehind(worktreePath string) (ahead, behind int) {
	cmd := exec.Command("git", "-C", worktreePath, "rev-list", "--left-right", "--count",
		"HEAD..."+"origin/"+l.MainBranch)
	out, err := cmd.Output()
	if err != nil {
		return 0, 0
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) != 2 {
		return 0, 0
	}
	ahead = atoi(parts[0])
	behind = atoi(parts[1])
	return
}

// HasDB returns true if a postgres DB named DBName(name) exists locally.
// Connects to the maintenance "postgres" database so psql doesn't fail
// when the user's $USER database doesn't exist.
func HasDB(name string) bool {
	cmd := exec.Command("psql", "-d", "postgres", "-Atc",
		"SELECT 1 FROM pg_database WHERE datname='"+DBName(name)+"'")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "1"
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
