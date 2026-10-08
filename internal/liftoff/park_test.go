package liftoff

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParkStopsServicesAndPreservesWorkspaceUntilResume(t *testing.T) {
	setStateDir(t)
	setRunDir(t)
	shortStopTimeouts(t)
	name := uniqueWorktree(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "uncommitted.txt")
	if err := os.WriteFile(file, []byte("keep my work"), 0600); err != nil {
		t.Fatal(err)
	}
	original := WorktreeMeta{Slot: 3, Path: dir, Branch: "wip", DatabaseName: "private_db", HerdrID: "herdr-id", RexID: "rex-id"}
	if err := WithConfigLock(func(c *Config) error { c.Worktrees[name] = original; return nil }); err != nil {
		t.Fatal(err)
	}
	pid := startHelper(t, serviceTag(name, SvcApp))
	layout := Layout{Root: filepath.Dir(dir), Master: filepath.Join(t.TempDir(), "master")}
	if err := layout.Park(name); err != nil {
		t.Fatal(err)
	}
	if IsAlive(pid) {
		t.Fatal("park left the disposable service running")
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	meta := cfg.Worktrees[name]
	if !meta.Parked {
		t.Fatal("park did not persist parked state")
	}
	meta.Parked = false
	if !reflect.DeepEqual(meta, original) {
		t.Fatalf("park changed resource ownership: %+v", meta)
	}
	if b, err := os.ReadFile(file); err != nil || string(b) != "keep my work" {
		t.Fatalf("park changed local files: %q, %v", b, err)
	}
	if err := layout.Resume(name); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Worktrees[name], original) {
		t.Fatalf("resume did not restore visibility without changing resources: %+v", cfg.Worktrees[name])
	}
}

func TestParkRejectsMasterAndUnregisteredWorkspaces(t *testing.T) {
	setStateDir(t)
	layout := Layout{Master: t.TempDir()}
	if err := layout.Park("master"); err == nil {
		t.Fatal("master must not be parked")
	}
	if err := layout.Park("unregistered"); err == nil {
		t.Fatal("unregistered workspace must not be parked")
	}
	if err := WithConfigLock(func(c *Config) error {
		c.Worktrees["master-alias"] = WorktreeMeta{Path: layout.Master}
		c.Worktrees["recovering"] = WorktreeMeta{CleanupPending: true}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"master-alias", "recovering"} {
		if err := layout.Park(name); err == nil {
			t.Fatalf("park accepted %s", name)
		}
	}
}

func TestParkKeepsWorkspaceVisibleWhenServiceStopFails(t *testing.T) {
	setStateDir(t)
	setRunDir(t)
	name := uniqueWorktree(t)
	if err := WithConfigLock(func(c *Config) error { c.Worktrees[name] = WorktreeMeta{Slot: 2}; return nil }); err != nil {
		t.Fatal(err)
	}
	pidFile, err := PIDFile(name, string(SvcApp))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pidFile, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pidFile, "block-removal"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := (Layout{}).Park(name); err == nil {
		t.Fatal("expected service stop failure")
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Worktrees[name].Parked {
		t.Fatal("failed park hid workspace")
	}
}
