package liftoff

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveImportedKitMappingPrefersHerdrIdentity(t *testing.T) {
	setStateDir(t)
	root := t.TempDir()
	c := &Config{Settings: Settings{Root: t.TempDir()}, Worktrees: map[string]WorktreeMeta{
		"primary": {HerdrID: "w66", Path: root},
	}}
	if err := writeConfigForImportTest(c); err != nil {
		t.Fatal(err)
	}
	if err := saveImportedKitMapping(HerdrWorkspace{WorkspaceID: "w67", Worktree: &HerdrWorktreeRef{CheckoutPath: root}}, "rex-secondary"); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.Worktrees["primary"].RexID != "" {
		t.Fatalf("secondary source overwrote primary mapping: %q", got.Worktrees["primary"].RexID)
	}
}

func TestSaveImportedKitMappingRelativeMasterAndRootWindowRole(t *testing.T) {
	setStateDir(t)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "master"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KIT_ROOT", root)
	t.Setenv("KIT_MASTER_DIR", "master")
	c := &Config{Settings: Settings{Root: root, MasterDir: "master"}, Worktrees: map[string]WorktreeMeta{}}
	if err := writeConfigForImportTest(c); err != nil {
		t.Fatal(err)
	}
	ws := HerdrWorkspace{WorkspaceID: "master-source", Worktree: &HerdrWorktreeRef{CheckoutPath: filepath.Join(root, "master")}}
	if err := saveImportedKitMapping(ws, "rex-master", "window-root"); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.Settings.RexMasterSession != "rex-master" || got.Settings.RexMasterShellWindow != "window-root" {
		t.Fatalf("master role mapping = %#v", got.Settings)
	}
}

func writeConfigForImportTest(c *Config) error {
	return WithConfigLock(func(current *Config) error { *current = *c; return nil })
}
