package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSemverNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v0.1.7", "v0.1.6", true},    // genuine update
		{"v0.1.5", "v0.1.6", false},   // stale cache behind us — the reported bug
		{"v0.1.6", "v0.1.6", false},   // equal
		{"v0.2.0", "v0.1.9", true},    // minor bump
		{"v1.0.0", "v0.9.9", true},    // major bump
		{"v0.1.10", "v0.1.9", true},   // numeric, not lexical
		{"v0.1.6.1", "v0.1.6", true},  // 4-part hotfix > 3-part base
		{"v0.1.6", "v0.1.6.1", false}, // base < its hotfix
		{"v0.1.6.2", "v0.1.6.1", true},
		{"", "v0.1.6", false}, // empty cache → no nag
		{"garbage", "v0.1.6", false},
	}
	for _, c := range cases {
		if got := semverNewer(c.a, c.b); got != c.want {
			t.Errorf("semverNewer(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestFetchReleaseTagFromAnnotatedTag(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	repo := t.TempDir()
	runTestGit(t, repo, "init", "--quiet")
	runTestGit(t, repo, "config", "user.email", "test@example.com")
	runTestGit(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repo, "add", ".")
	runTestGit(t, repo, "commit", "--quiet", "-m", "init")
	commit := runTestGit(t, repo, "rev-parse", "HEAD")
	runTestGit(t, repo, "tag", "-a", "v9.9.9", "-m", "test tag")
	if got := runTestGit(t, repo, "cat-file", "-t", "v9.9.9"); got != "tag" {
		t.Fatalf("test setup made %q tag, want annotated tag object", got)
	}

	dst := t.TempDir()
	if err := fetchReleaseTagFrom(context.Background(), repo, "v9.9.9", dst); err != nil {
		t.Fatal(err)
	}
	if got := runTestGit(t, dst, "rev-parse", "HEAD"); got != commit {
		t.Fatalf("checked out %s, want %s", got, commit)
	}
	if branch := runTestGit(t, dst, "branch", "--show-current"); branch != "" {
		t.Fatalf("branch = %q, want detached HEAD", branch)
	}
}

func runTestGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}
