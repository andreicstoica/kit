package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andreicstoica/kit/internal/liftoff"
)

func TestRexDirectActionResumesExactSessionAndLogsResult(t *testing.T) {
	t.Setenv("KIT_STATE_DIR", t.TempDir())
	t.Setenv("KIT_RUN_DIR", t.TempDir())
	oldSession, oldNotify := rexContextSession, rexActionNotify
	t.Cleanup(func() { rexContextSession, rexActionNotify = oldSession, oldNotify })
	rexContextSession, rexActionNotify = "owned-session", false
	if err := liftoff.WithConfigLock(func(c *liftoff.Config) error {
		c.Worktrees["selected-workspace"] = liftoff.WorktreeMeta{RexID: "owned-session", Parked: true, DatabaseName: "keep-db", Slot: 3}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	rexActionCmd.SetOut(&out)
	t.Cleanup(func() { rexActionCmd.SetOut(nil) })
	if err := rexActionCmd.RunE(rexActionCmd, []string{"resume"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := liftoff.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	meta := cfg.Worktrees["selected-workspace"]
	if meta.Parked || meta.DatabaseName != "keep-db" || meta.Slot != 3 {
		t.Fatalf("resume changed resources: %+v", meta)
	}
	if !strings.Contains(out.String(), "state=done") {
		t.Fatalf("no completion status: %s", out.String())
	}
	log, err := os.ReadFile(filepath.Join(liftoff.RunDirPath("selected-workspace"), "rex-operation.log"))
	if err != nil || !strings.Contains(string(log), "resume finished") {
		t.Fatalf("missing operation log: %s, %v", log, err)
	}
}

func TestRexDirectPlayOfParkedWorkspaceReportsErrorWithoutStarting(t *testing.T) {
	t.Setenv("KIT_STATE_DIR", t.TempDir())
	t.Setenv("KIT_RUN_DIR", t.TempDir())
	oldSession, oldNotify := rexContextSession, rexActionNotify
	t.Cleanup(func() { rexContextSession, rexActionNotify = oldSession, oldNotify })
	rexContextSession, rexActionNotify = "parked-session", false
	if err := liftoff.WithConfigLock(func(c *liftoff.Config) error {
		c.Worktrees["parked-workspace"] = liftoff.WorktreeMeta{RexID: "parked-session", Parked: true}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	rexActionCmd.SetOut(&out)
	t.Cleanup(func() { rexActionCmd.SetOut(nil) })
	if err := rexActionCmd.RunE(rexActionCmd, []string{"play"}); err == nil {
		t.Fatal("play must require resume")
	}
	if !strings.Contains(out.String(), "state=error") {
		t.Fatalf("direct action failed silently: %s", out.String())
	}
}
