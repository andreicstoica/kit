package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andreicstoica/kit/internal/liftoff"
)

func TestLineupFiltersParkedWorkspacesInTableAndTree(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "master")
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KIT_STATE_DIR", filepath.Join(dir, "state"))
	t.Setenv("KIT_RUN_DIR", filepath.Join(dir, "run"))
	t.Setenv("KIT_WORKSPACE_BACKEND", "herdr")
	t.Setenv("PATH", bin)
	porcelain := "worktree " + root + "\nHEAD abc\nbranch refs/heads/master\n\nworktree " + filepath.Join(dir, "active-feature") + "\nHEAD abc\nbranch refs/heads/active-feature\n\nworktree " + filepath.Join(dir, "parked-feature") + "\nHEAD abc\nbranch refs/heads/parked-feature\n\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\ncase \"$*\" in *'worktree list'*) printf '%s' '"+porcelain+"';; esac\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := liftoff.WithConfigLock(func(c *liftoff.Config) error {
		c.Worktrees["active-feature"] = liftoff.WorktreeMeta{Slot: 1}
		c.Worktrees["parked-feature"] = liftoff.WorktreeMeta{Slot: 2, Parked: true}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	layout := liftoff.Layout{Root: dir, Master: root}
	for _, render := range []func(liftoff.Layout) (string, error){RenderLineup, RenderLineupTree} {
		out, err := render(layout)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "active-feature") || strings.Contains(out, "parked-feature") {
			t.Fatalf("default lineup visibility wrong: %s", out)
		}
	}
	for _, render := range []func(liftoff.Layout) (string, error){RenderLineupParked, RenderLineupTreeParked} {
		out, err := render(layout)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "parked-feature") || strings.Contains(out, "active-feature") {
			t.Fatalf("parked lineup visibility wrong: %s", out)
		}
	}
}
