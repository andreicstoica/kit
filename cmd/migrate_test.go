package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andreicstoica/kit/internal/liftoff"
)

func TestMigrateRexApplyRejectsDuplicateLabelsBeforeMutation(t *testing.T) {
	t.Setenv("KIT_STATE_DIR", t.TempDir())
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "rex"), []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	oldHerdr, oldRex, oldApply := readHerdrForRexImport, readRexForImport, applyRexImport
	oldDoApply, oldDry, oldFilter := rexMigrateApply, rexMigrateDryRun, rexMigrateWorkspace
	t.Cleanup(func() {
		readHerdrForRexImport, readRexForImport, applyRexImport = oldHerdr, oldRex, oldApply
		rexMigrateApply, rexMigrateDryRun, rexMigrateWorkspace = oldDoApply, oldDry, oldFilter
		migrateRexCmd.SetOut(nil)
	})
	root := t.TempDir()
	readHerdrForRexImport = func() (liftoff.HerdrState, error) {
		return liftoff.HerdrState{Workspaces: []liftoff.HerdrWorkspace{{WorkspaceID: "one", Label: "duplicate", Worktree: &liftoff.HerdrWorktreeRef{CheckoutPath: root}}, {WorkspaceID: "two", Label: "duplicate", Worktree: &liftoff.HerdrWorktreeRef{CheckoutPath: root}}}}, nil
	}
	readRexForImport = func() (liftoff.RexState, error) { return liftoff.RexState{}, nil }
	called := false
	applyRexImport = func(liftoff.HerdrState, *liftoff.RexImportMap) error { called = true; return nil }
	rexMigrateApply, rexMigrateDryRun, rexMigrateWorkspace = true, false, ""
	migrateRexCmd.SetOut(io.Discard)
	if err := migrateRexCmd.RunE(migrateRexCmd, nil); err == nil {
		t.Fatal("duplicate-label import must be refused")
	}
	if called {
		t.Fatal("refused plan reached the mutating import boundary")
	}
}

func TestMigrateRexDryRunAndIdempotentApplyWithFakeCLIs(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	stateDir := filepath.Join(dir, "state")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	checkout := filepath.Join(dir, "checkout")
	if err := os.Mkdir(checkout, 0o755); err != nil {
		t.Fatal(err)
	}
	herdr := `#!/bin/sh
cat <<'JSON'
 {"workspaces":[{"workspace_id":"herdr-w1","label":"feature-one","worktree":{"checkout_path":"` + checkout + `"}}],"tabs":[{"tab_id":"herdr-t1","workspace_id":"herdr-w1","label":"shell"},{"tab_id":"herdr-t2","workspace_id":"herdr-w1","label":"logs"}],"panes":[{"pane_id":"herdr-p1","tab_id":"herdr-t1","workspace_id":"herdr-w1","cwd":"` + checkout + `"},{"pane_id":"herdr-p2","tab_id":"herdr-t1","workspace_id":"herdr-w1","cwd":"` + checkout + `"}]}
JSON
`
	rex := `#!/bin/sh
if [ "$1" = "--autostart=false" ]; then shift; fi
marker="` + filepath.Join(dir, "created") + `"
count="` + filepath.Join(dir, "new-count") + `"
tab2="` + filepath.Join(dir, "tab2-created") + `"
failed="` + filepath.Join(dir, "tab2-failed-once") + `"
splitCount="` + filepath.Join(dir, "split-count") + `"
if [ "$1" = "new" ]; then
  n=0; [ -f "$count" ] && n=$(cat "$count"); n=$((n+1)); echo "$n" > "$count"; touch "$marker"; echo '{"session_id":"rex-s1"}'; exit 0
fi
if [ "$1" = "do" ]; then
  case "$*" in
    *'local result = {sessions={}}'*)
      if [ -f "$marker" ]; then
        blocks='{"block_id":"rex-block1"}'
        [ -f "$splitCount" ] && blocks=$blocks',{"block_id":"rex-block2"}'
        if [ -f "$tab2" ]; then printf '{"sessions":[{"session_id":"rex-s1","label":"feature-one","windows":[{"window_id":"rex-win1","label":"shell","blocks":[%s]},{"window_id":"rex-win2","label":"logs","blocks":[{"block_id":"rex-block3"}]}]}]}\n' "$blocks"; else printf '{"sessions":[{"session_id":"rex-s1","label":"feature-one","windows":[{"window_id":"rex-win1","label":"shell","blocks":[%s]}]}]}\n' "$blocks"; fi
      else echo '{"sessions":[]}'; fi
      ;;
    *'new_split'*) n=0; [ -f "$splitCount" ] && n=$(cat "$splitCount"); n=$((n+1)); echo "$n" > "$splitCount"; echo '{"block_id":"rex-block2"}' ;;
    *'new_window'*)
      if [ ! -f "$failed" ]; then touch "$failed"; echo 'simulated interrupted import' >&2; exit 1; fi
      touch "$tab2"; echo '{"window_id":"rex-win2","block_ids":["rex-block3"]}'
      ;;
    *'set_window_label'*) echo 'true' ;;
    *) echo '{"window_id":"rex-win1","block_ids":["rex-block1"]}' ;;
  esac
  exit 0
fi
echo "unexpected fake rex invocation: $*" >&2; exit 2
`
	writeExec := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeExec(filepath.Join(bin, "herdr"), herdr)
	writeExec(filepath.Join(bin, "rex"), rex)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KIT_STATE_DIR", stateDir)
	t.Setenv("HERDR_SESSION", "test")
	oldApply := rexMigrateApply
	oldDryRun, oldWorkspace := rexMigrateDryRun, rexMigrateWorkspace
	defer func() { rexMigrateApply = oldApply; rexMigrateDryRun = oldDryRun; rexMigrateWorkspace = oldWorkspace }()
	cmd := migrateRexCmd
	var output strings.Builder
	cmd.SetOut(&output)
	rexMigrateApply = false
	rexMigrateDryRun = true
	rexMigrateWorkspace = "herdr-w1"
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "DRY RUN") || strings.Contains(output.String(), "APPLY —") {
		t.Fatalf("unexpected dry run output: %s", output.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "created")); !os.IsNotExist(err) {
		t.Fatal("dry-run created a Rex session")
	}
	if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
		t.Fatal("dry-run wrote import/config state")
	}

	rexMigrateApply = true
	rexMigrateDryRun = false
	output.Reset()
	if err := cmd.RunE(cmd, nil); err == nil || !strings.Contains(err.Error(), "simulated interrupted import") {
		t.Fatalf("expected simulated partial failure, got %v", err)
	}
	firstMap, err := os.ReadFile(filepath.Join(stateDir, "rex-import.json"))
	if err != nil || !strings.Contains(string(firstMap), "herdr-t1") || !strings.Contains(string(firstMap), "herdr-p1") || !strings.Contains(string(firstMap), "herdr-p2") || strings.Contains(string(firstMap), "herdr-t2") {
		t.Fatalf("unexpected partial mapping: %s %v", firstMap, err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	count, err := os.ReadFile(filepath.Join(dir, "new-count"))
	if err != nil || strings.TrimSpace(string(count)) != "1" {
		t.Fatalf("Rex sessions created more than once: %q %v", count, err)
	}
	splitCount, err := os.ReadFile(filepath.Join(dir, "split-count"))
	if err != nil || strings.TrimSpace(string(splitCount)) != "1" {
		t.Fatalf("split duplicated on retry: %q %v", splitCount, err)
	}
	info, err := os.Stat(filepath.Join(stateDir, "rex-import.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("import map permissions = %o", info.Mode().Perm())
	}
}
