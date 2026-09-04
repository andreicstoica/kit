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
