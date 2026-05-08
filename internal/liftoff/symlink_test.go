package liftoff

import (
	"os"
	"path/filepath"
	"testing"
)

func setupSymlinkTest(t *testing.T) (master, worktree string) {
	t.Helper()
	root := t.TempDir()
	master = filepath.Join(root, "master")
	worktree = filepath.Join(root, "wt")
	for _, sub := range nodeModulesDirs {
		if err := os.MkdirAll(filepath.Join(master, sub, "node_modules", "react"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(master, sub, "package.json"), []byte(`{"v":1}`), 0o644); err != nil {
			t.Fatal(err)
		}
		// worktree dirs exist (simulating git worktree add) but with empty node_modules.
		if err := os.MkdirAll(filepath.Join(worktree, sub, "node_modules"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(worktree, sub, "package.json"), []byte(`{"v":1}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return master, worktree
}

func TestLinkNodeModules_FreshLink(t *testing.T) {
	master, worktree := setupSymlinkTest(t)
	results, err := LinkNodeModules(master, worktree, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != len(nodeModulesDirs) {
		t.Fatalf("results = %d, want %d", len(results), len(nodeModulesDirs))
	}
	for _, r := range results {
		if r.Action != "linked" {
			t.Errorf("expected linked, got %q for %s", r.Action, r.Path)
		}
	}
	// Verify symlink target.
	target, err := os.Readlink(filepath.Join(worktree, "frontend/app/node_modules"))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(master, "frontend/app/node_modules")
	if target != want {
		t.Errorf("symlink → %s, want %s", target, want)
	}
}

func TestLinkNodeModules_KeepExistingCorrectLink(t *testing.T) {
	master, worktree := setupSymlinkTest(t)
	// First call: creates the symlinks.
	if _, err := LinkNodeModules(master, worktree, nil); err != nil {
		t.Fatal(err)
	}
	// Second call: should keep them.
	results, err := LinkNodeModules(master, worktree, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Action != "kept" {
			t.Errorf("expected kept, got %q", r.Action)
		}
	}
}

func TestLinkNodeModules_NoMasterSource(t *testing.T) {
	root := t.TempDir()
	master := filepath.Join(root, "master")
	worktree := filepath.Join(root, "wt")
	for _, sub := range nodeModulesDirs {
		if err := os.MkdirAll(filepath.Join(worktree, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Master has no node_modules.
	if err := os.MkdirAll(master, 0o755); err != nil {
		t.Fatal(err)
	}
	results, err := LinkNodeModules(master, worktree, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Action == "linked" {
			t.Errorf("should not link without source: %s", r.Path)
		}
	}
}

func TestLinkNodeModules_StaleLockfile(t *testing.T) {
	master, worktree := setupSymlinkTest(t)
	// Simulate worktree's package.json drifting.
	if err := os.WriteFile(filepath.Join(worktree, "frontend/app/package.json"), []byte(`{"v":2}`), 0o644); err != nil {
		t.Fatal(err)
	}
	results, err := LinkNodeModules(master, worktree, nil)
	if err != nil {
		t.Fatal(err)
	}
	saw := false
	for _, r := range results {
		if r.StaleLockfile {
			saw = true
		}
	}
	if !saw {
		t.Errorf("expected at least one StaleLockfile=true result")
	}
}
