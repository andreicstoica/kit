package liftoff

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultLayoutEnvOverride(t *testing.T) {
	t.Setenv("KIT_ROOT", "/tmp/test-liftoff")
	t.Setenv("KIT_MASTER_DIR", "the-master")
	t.Setenv("KIT_GTAB_DIR", "/tmp/gtab")
	t.Setenv("KIT_MAIN_BRANCH", "trunk")
	l := DefaultLayout()
	if l.Root != "/tmp/test-liftoff" {
		t.Errorf("Root = %q", l.Root)
	}
	if l.Master != "/tmp/test-liftoff/the-master" {
		t.Errorf("Master = %q", l.Master)
	}
	if l.GtabDir != "/tmp/gtab" {
		t.Errorf("GtabDir = %q", l.GtabDir)
	}
	if l.MainBranch != "trunk" {
		t.Errorf("MainBranch = %q", l.MainBranch)
	}
}

func TestWorktreePaths(t *testing.T) {
	t.Setenv("KIT_ROOT", "/r")
	t.Setenv("KIT_MASTER_DIR", "m")
	l := DefaultLayout()
	if got := l.WorktreePath("foo"); got != filepath.Join("/r", "foo") {
		t.Errorf("WorktreePath = %q", got)
	}
	if got := l.LegacyWorktreePath("foo"); got != filepath.Join("/r", "liftoff-foo") {
		t.Errorf("LegacyWorktreePath = %q", got)
	}
}

func TestGtabFile(t *testing.T) {
	t.Setenv("KIT_GTAB_DIR", "/g")
	l := DefaultLayout()
	if got := l.GtabFile("foo"); got != "/g/foo.applescript" {
		t.Errorf("GtabFile = %q", got)
	}
}

func TestResolveWorktreePathPrefersPersistedMetadata(t *testing.T) {
	setStateDir(t)
	root := t.TempDir()
	stored := filepath.Join(t.TempDir(), "custom-checkout")
	if err := os.Mkdir(stored, 0o755); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	c.Worktrees["feature-a"] = WorktreeMeta{Path: stored, Adopted: true}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	layout := Layout{Root: root, Master: filepath.Join(root, "master")}
	got, err := layout.ResolveWorktreePath("feature-a")
	if err != nil {
		t.Fatal(err)
	}
	if got != stored {
		t.Fatalf("ResolveWorktreePath = %q, want persisted %q", got, stored)
	}
}

func TestResolveWorktreePathFallsBackWhenPersistedPathMissing(t *testing.T) {
	setStateDir(t)
	root := t.TempDir()
	canonical := filepath.Join(root, "feature-a")
	if err := os.Mkdir(canonical, 0o755); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	c.Worktrees["feature-a"] = WorktreeMeta{Path: filepath.Join(root, "missing")}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	layout := Layout{Root: root, Master: filepath.Join(root, "master")}
	got, err := layout.ResolveWorktreePath("feature-a")
	if err != nil {
		t.Fatal(err)
	}
	if got != canonical {
		t.Fatalf("ResolveWorktreePath = %q, want canonical %q", got, canonical)
	}
}
