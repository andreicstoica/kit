package liftoff

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindOrphanedWorktrees_DetectsMissingCheckout(t *testing.T) {
	l := newMasterRepo(t)
	setStateDir(t)
	t.Setenv("KIT_RUN_DIR", t.TempDir())
	l.GtabDir = t.TempDir()

	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "pg_dump"), "#!/bin/sh\nexit 0\n")
	writeExecutable(t, filepath.Join(bin, "psql"), "#!/bin/sh\nprintf '1\\n'\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Worktrees["orphan"] = WorktreeMeta{
		Slot:   4,
		Branch: "feature/orphan",
		Path:   filepath.Join(filepath.Dir(l.Master), "orphan"),
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.GtabFile("orphan"), []byte("layout"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(RunDirPath("orphan"), 0o755); err != nil {
		t.Fatal(err)
	}

	candidates, err := l.FindOrphanedWorktrees()
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("orphan candidates = %+v, want one candidate", candidates)
	}
	candidate := candidates[0]
	if candidate.Name != "orphan" || !candidate.HasDB || !candidate.HasGtab || !candidate.HasRunDir {
		t.Fatalf("orphan candidate = %+v, want all durable resources", candidate)
	}
}

func TestReconcileOrphan_RemovesMissingGitRecordKeepsBranch(t *testing.T) {
	l := newMasterRepo(t)
	setStateDir(t)
	t.Setenv("KIT_RUN_DIR", t.TempDir())
	l.GtabDir = t.TempDir()
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "psql"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	path := addWorktree(t, l, "lost")
	writeFile(t, path, "work.txt", "keep this commit")
	runGit(t, path, "add", ".")
	runGit(t, path, "commit", "-m", "local work")
	head := runGit(t, path, "rev-parse", "HEAD")
	if err := WithConfigLock(func(c *Config) error {
		c.Worktrees["lost"] = WorktreeMeta{Slot: 4, Path: path, Branch: "lost"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	candidates, err := l.FindOrphanedWorktrees()
	if err != nil || len(candidates) != 1 {
		t.Fatalf("missing checkout candidates = %+v, %v; want one", candidates, err)
	}
	if err := l.ReconcileOrphan(candidates[0], nil); err != nil {
		t.Fatal(err)
	}
	wts, err := l.ListWorktrees()
	if err != nil || len(wts) != 1 {
		t.Fatalf("worktrees after cleanup = %+v, %v; want master only", wts, err)
	}
	if got := runGit(t, l.Master, "rev-parse", "lost"); got != head {
		t.Fatalf("retained branch = %s, want %s", got, head)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Worktrees["lost"]; ok {
		t.Fatal("orphan config record retained after successful cleanup")
	}
}

func TestFindOrphanedWorktrees_PreservesMovedAndLockedCheckouts(t *testing.T) {
	for _, mode := range []string{"moved", "locked"} {
		t.Run(mode, func(t *testing.T) {
			l := newMasterRepo(t)
			setStateDir(t)
			path := addWorktree(t, l, "keep")
			if err := WithConfigLock(func(c *Config) error {
				c.Worktrees["keep"] = WorktreeMeta{Slot: 4, Path: path, Branch: "keep"}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if mode == "moved" {
				runGit(t, l.Master, "worktree", "move", path, path+"-moved")
			} else {
				runGit(t, l.Master, "worktree", "lock", "--reason", "external disk", path)
				if err := os.RemoveAll(path); err != nil {
					t.Fatal(err)
				}
			}
			got, err := l.FindOrphanedWorktrees()
			if err != nil || len(got) != 0 {
				t.Fatalf("protected checkout candidates = %+v, %v; want none", got, err)
			}
		})
	}
}
