package liftoff

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runGit runs git in dir and fails the test on error. Shared by the
// git-backed tests in this package.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newMasterRepo creates a master repo with one commit on `master`, wired to a
// local bare "origin" remote (so branches can be pushed to set upstreams), and
// returns a Layout pointing at it. Skips the test if git is unavailable.
func newMasterRepo(t *testing.T) Layout {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	root := t.TempDir()
	// On macOS /tmp is a symlink to /private/tmp; git reports the resolved
	// path in `worktree list`, so canonicalize root to match for IsMaster.
	if rp, err := filepath.EvalSymlinks(root); err == nil {
		root = rp
	}
	master := filepath.Join(root, "master")
	remote := filepath.Join(root, "remote.git")
	if err := os.MkdirAll(master, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatal(err)
	}

	runGit(t, remote, "init", "--bare", "-b", "master")
	runGit(t, master, "init", "-b", "master")
	runGit(t, master, "config", "user.email", "test@example.com")
	runGit(t, master, "config", "user.name", "Test")
	runGit(t, master, "config", "commit.gpgsign", "false")
	writeFile(t, master, "README.md", "init")
	runGit(t, master, "add", ".")
	runGit(t, master, "commit", "-m", "init")
	runGit(t, master, "remote", "add", "origin", remote)
	runGit(t, master, "push", "-u", "origin", "master")

	return Layout{Master: master, MainBranch: "master"}
}

// addWorktree adds a worktree at <root>/<name> on a fresh branch off master.
func addWorktree(t *testing.T, l Layout, name string) string {
	t.Helper()
	path := filepath.Join(filepath.Dir(l.Master), name)
	runGit(t, l.Master, "worktree", "add", path, "-b", name, "master")
	return path
}
