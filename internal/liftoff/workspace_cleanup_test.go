package liftoff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCloseHerdrDoesNotReplaceStaleIDWithSameLabel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KIT_STATE_DIR", filepath.Join(dir, "state"))
	t.Setenv("KIT_WORKSPACE_BACKEND", "rex")
	t.Setenv("HERDR_SESSION", "isolated")
	log := filepath.Join(dir, "calls")
	t.Setenv("KIT_CLOSE_CALLS", log)
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$KIT_CLOSE_CALLS"
case "$*" in
  "api snapshot") printf '%s\n' '{"workspaces":[{"workspace_id":"unrelated","label":"feature-a"}],"tabs":[],"panes":[]}' ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "herdr"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if err := WithConfigLock(func(c *Config) error {
		c.Worktrees["feature-a"] = WorktreeMeta{HerdrID: "gone", HerdrSpace: "feature-a"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := CloseManagedWorkspaces("feature-a", "/work/feature-a"); err != nil {
		t.Fatal(err)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(calls), "workspace close") {
		t.Fatalf("closed unrelated workspace after its saved ID disappeared: %s", calls)
	}
}

func TestCloseHerdrDoesNotResurrectDeletedKitRecord(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KIT_STATE_DIR", filepath.Join(dir, "state"))
	t.Setenv("PATH", dir)
	if err := os.WriteFile(filepath.Join(dir, "herdr"), []byte("#!/bin/sh\nprintf '%s\\n' '{\"workspaces\":[],\"tabs\":[],\"panes\":[]}'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := CloseHerdr("missing", "/missing"); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Worktrees["missing"]; ok {
		t.Fatal("close recreated a worktree record that did not exist")
	}
}
