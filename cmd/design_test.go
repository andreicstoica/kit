package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/spf13/cobra"
)

func headlessDesignFixture(t *testing.T) liftoff.Layout {
	t.Helper()
	layout := newSyncMasterRepo(t)
	layout.Root = filepath.Dir(layout.Master)
	layout.GtabDir = filepath.Join(t.TempDir(), "gtab")
	t.Setenv("KIT_ROOT", layout.Root)
	t.Setenv("KIT_MASTER_DIR", filepath.Base(layout.Master))
	t.Setenv("KIT_MAIN_BRANCH", layout.MainBranch)
	t.Setenv("KIT_GTAB_DIR", layout.GtabDir)
	t.Setenv("KIT_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	t.Setenv("KIT_RUN_DIR", t.TempDir())
	if err := os.MkdirAll(filepath.Join(layout.Master, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(layout.Master, "backend"), ".env", "SQLALCHEMY_DATABASE_NAME=liftoff\n")
	runTestGit(t, layout.Master, "add", ".")
	runTestGit(t, layout.Master, "commit", "-m", "backend settings")
	runTestGit(t, layout.Master, "push", "origin", "master")
	return layout
}

type designWireOutput struct {
	Name            string         `json:"name"`
	Path            string         `json:"path"`
	Branch          string         `json:"branch"`
	DatabaseMode    string         `json:"database_mode"`
	BackendDeps     bool           `json:"backend_deps"`
	SymlinkFrontend bool           `json:"symlink_frontend"`
	DryRun          bool           `json:"dry_run"`
	Slot            int            `json:"slot"`
	Ports           map[string]int `json:"ports"`
}

func designTestCommand(t *testing.T, args ...string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	root := &cobra.Command{
		Use: "kit", SilenceErrors: true, SilenceUsage: true,
		PersistentPreRunE: func(*cobra.Command, []string) error {
			t.Error("headless design ran the default setup/update hook")
			return errors.New("unexpected startup hook")
		},
	}
	root.AddCommand(newDesignCmd())
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(append([]string{"design"}, args...))
	return root, &stdout, &stderr
}

func decodeDesignOutput(t *testing.T, data []byte) designWireOutput {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("stdout must contain one JSON object: %v\n%s", err, data)
	}
	for _, key := range []string{"name", "path", "branch", "database_mode", "backend_deps", "symlink_frontend", "graphite", "checkout_backend", "allow_pg_dump", "dry_run"} {
		if _, exists := fields[key]; !exists {
			t.Errorf("JSON result omitted %q", key)
		}
	}
	var output designWireOutput
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	return output
}

func TestHeadlessDesignDryRunIsReadOnly(t *testing.T) {
	layout := headlessDesignFixture(t)
	cmd, stdout, stderr := designTestCommand(t, "Liftoff-Login-Fix", "--dry-run", "--json", "--graphite=false")
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	result := decodeDesignOutput(t, stdout.Bytes())
	if result.Name != "login-fix" || result.Branch != "login-fix" || result.Path != layout.WorktreePath("login-fix") || !result.DryRun {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.DatabaseMode != "none" || result.BackendDeps || !result.SymlinkFrontend || result.Slot != 0 || result.Ports != nil {
		t.Fatalf("unsafe defaults: %+v", result)
	}
	if stderr.Len() != 0 {
		t.Fatalf("dry run emitted progress: %s", stderr.String())
	}
	if _, err := os.Stat(result.Path); !os.IsNotExist(err) {
		t.Fatalf("dry run created worktree: %v", err)
	}
	if _, err := os.Stat(liftoff.ConfigPath()); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote config: %v", err)
	}
	if branches := runTestGit(t, layout.Master, "branch", "--list", "login-fix"); branches != "" {
		t.Fatalf("dry run created branch: %s", branches)
	}
}

func TestHeadlessDesignCreatesDeferredWorkspaceWithSingleJSONResult(t *testing.T) {
	layout := headlessDesignFixture(t)
	cmd, stdout, stderr := designTestCommand(t, "login-fix", "--json", "--graphite=false", "--symlink-frontend=false")
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	result := decodeDesignOutput(t, stdout.Bytes())
	if result.DryRun || result.Slot < 1 || result.Ports == nil || result.Ports["api"] == 0 || result.Ports["slot"] != result.Slot {
		t.Fatalf("missing allocated ports: %+v", result)
	}
	if !strings.Contains(stderr.String(), "fetch origin/master") {
		t.Fatalf("progress missing from stderr: %s", stderr.String())
	}
	cfg, err := liftoff.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	meta := cfg.Worktrees["login-fix"]
	if !meta.DatabaseDeferred || meta.DatabaseName != "" || meta.Slot != result.Slot {
		t.Fatalf("invalid deferred state: %+v", meta)
	}
	data, err := os.ReadFile(filepath.Join(result.Path, "backend", ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "SQLALCHEMY_DATABASE_NAME=liftoff_login_fix") {
		t.Fatalf("shared database inherited: %s", data)
	}
	if _, err := os.Stat(layout.GtabFile("login-fix")); !os.IsNotExist(err) {
		t.Fatalf("headless design wrote terminal layout: %v", err)
	}
}

func TestHeadlessDesignRejectsInvalidRequestsBeforeCreation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		flags []string
	}{
		{name: "missing name"},
		{name: "reserved name", args: []string{"master"}},
		{name: "path traversal", args: []string{"../outside"}},
		{name: "database identifier overflow", args: []string{strings.Repeat("a", 56)}},
		{name: "bad mode", args: []string{"feature"}, flags: []string{"--db=shared"}},
		{name: "empty mode", args: []string{"feature"}, flags: []string{"--db="}},
		{name: "unrelated dump permission", args: []string{"feature"}, flags: []string{"--allow-pg-dump"}},
		{name: "bad checkout", args: []string{"feature"}, flags: []string{"--checkout-backend=other"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			headlessDesignFixture(t)
			cmd, stdout, _ := designTestCommand(t, append(tc.args, tc.flags...)...)
			if err := cmd.Execute(); err == nil {
				t.Fatal("expected validation error")
			}
			if stdout.Len() != 0 {
				t.Fatalf("failure returned success output: %s", stdout.String())
			}
			if _, err := os.Stat(liftoff.ConfigPath()); !os.IsNotExist(err) {
				t.Fatalf("validation wrote config: %v", err)
			}
		})
	}
}

func TestHeadlessDesignPreservesExistingPathsAndRecords(t *testing.T) {
	for _, kind := range []string{"canonical", "legacy", "broken symlink", "record"} {
		t.Run(kind, func(t *testing.T) {
			layout := headlessDesignFixture(t)
			path := layout.WorktreePath("feature")
			switch kind {
			case "canonical", "legacy":
				if kind == "legacy" {
					path = layout.LegacyWorktreePath("feature")
				}
				if err := os.MkdirAll(path, 0o755); err != nil {
					t.Fatal(err)
				}
				writeTestFile(t, path, "keep", "user content")
			case "broken symlink":
				if err := os.Symlink(filepath.Join(t.TempDir(), "absent"), path); err != nil {
					t.Fatal(err)
				}
			case "record":
				cfg, err := liftoff.LoadConfig()
				if err != nil {
					t.Fatal(err)
				}
				cfg.Worktrees["feature"] = liftoff.WorktreeMeta{Slot: 9, CleanupPending: true}
				if err := cfg.Save(); err != nil {
					t.Fatal(err)
				}
			}
			cmd, _, _ := designTestCommand(t, "feature", "--json")
			if err := cmd.Execute(); err == nil {
				t.Fatal("existing user state should block creation")
			}
			if kind == "canonical" || kind == "legacy" {
				data, err := os.ReadFile(filepath.Join(path, "keep"))
				if err != nil || string(data) != "user content" {
					t.Fatalf("existing data changed: %q %v", data, err)
				}
			}
		})
	}
}

func TestHeadlessDesignConcurrentCreatesPreserveWinner(t *testing.T) {
	layout := headlessDesignFixture(t)
	first, _, _ := designTestCommand(t, "same-task", "--json", "--graphite=false", "--symlink-frontend=false")
	second, _, _ := designTestCommand(t, "same-task", "--json", "--graphite=false", "--symlink-frontend=false")
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, cmd := range []*cobra.Command{first, second} {
		go func() {
			<-start
			results <- cmd.Execute()
		}()
	}
	close(start)
	succeeded := 0
	for range 2 {
		if err := <-results; err == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("successful creates = %d, want 1", succeeded)
	}
	cfg, err := liftoff.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	meta := cfg.Worktrees["same-task"]
	if meta.Slot == 0 || !meta.DatabaseDeferred {
		t.Fatalf("losing create removed winner's state: %+v", meta)
	}
	if got := runTestGit(t, layout.WorktreePath("same-task"), "branch", "--show-current"); got != "same-task" {
		t.Fatalf("winner's checkout missing: %s", got)
	}
}

func TestHeadlessDesignFailureDoesNotReturnSuccessJSON(t *testing.T) {
	layout := headlessDesignFixture(t)
	runTestGit(t, layout.Master, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing.git"))
	cmd, stdout, _ := designTestCommand(t, "feature", "--json", "--graphite=false")
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected fetch failure")
	}
	if stdout.Len() != 0 {
		t.Fatalf("failed creation returned JSON: %s", stdout.String())
	}
	if _, err := os.Stat(layout.WorktreePath("feature")); !os.IsNotExist(err) {
		t.Fatalf("failed fetch left a worktree: %v", err)
	}
}
