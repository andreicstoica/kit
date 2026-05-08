package liftoff

import (
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
