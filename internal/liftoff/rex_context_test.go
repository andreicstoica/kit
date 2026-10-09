package liftoff

import (
	"path/filepath"
	"testing"
)

func TestRexWorkspaceUsesSavedIdentityNotLabelsOrCwd(t *testing.T) {
	setStateDir(t)
	root := t.TempDir()
	if err := WithConfigLock(func(c *Config) error {
		c.Settings.Root, c.Settings.MasterDir = root, "master"
		c.Settings.RexMasterSession = "master-session"
		c.Worktrees["actual-checkout"] = WorktreeMeta{RexID: "primary-session", Path: filepath.Join(root, "actual-checkout")}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := SaveRexImportMap(RexImportMap{Checkouts: map[string]string{"secondary-session": filepath.Join(root, "actual-checkout")}}); err != nil {
		t.Fatal(err)
	}
	for session, want := range map[string]string{"master-session": "master", "primary-session": "actual-checkout", "secondary-session": "actual-checkout"} {
		name, err := RexWorkspaceName(session)
		if err != nil || name != want {
			t.Fatalf("session %s = %q, %v; want %s", session, name, err, want)
		}
	}
	for _, session := range []string{"", "actual-checkout", "unmapped-session"} {
		if _, err := RexWorkspaceName(session); err == nil {
			t.Fatalf("accepted unowned session %q", session)
		}
	}
}
