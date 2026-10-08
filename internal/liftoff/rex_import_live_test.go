package liftoff

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRexImportDisposableLiveIntegration(t *testing.T) {
	runRexLiveImport(t, false)
}

func TestRexImportLiveRetryAfterSecondSplitFailure(t *testing.T) {
	runRexLiveImport(t, true)
}

func runRexLiveImport(t *testing.T, injectFailure bool) {
	t.Helper()
	if os.Getenv("KIT_TEST_REX") != "1" {
		t.Skip("set KIT_TEST_REX=1 to test a disposable real Rex import")
	}
	setStateDir(t)
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	name := "kit-import-test-" + filepath.Base(root)
	source := HerdrState{
		Workspaces: []HerdrWorkspace{{WorkspaceID: "source:live", Label: name, Worktree: &HerdrWorktreeRef{CheckoutPath: root}}},
		Tabs: []HerdrTab{
			{TabID: "source:shell", WorkspaceID: "source:live", Label: "work"},
			{TabID: "source:other", WorkspaceID: "source:live", Label: "other"},
		},
		Panes: []HerdrPane{
			{PaneID: "source:p1", TabID: "source:shell", WorkspaceID: "source:live", CWD: &nested},
			{PaneID: "source:p2", TabID: "source:shell", WorkspaceID: "source:live", CWD: &root},
			{PaneID: "source:p3", TabID: "source:other", WorkspaceID: "source:live", CWD: &root},
		},
	}
	expectedPanes := 3
	if injectFailure {
		expectedPanes = 4
		source.Panes = append(source.Panes, HerdrPane{PaneID: "source:p4", TabID: "source:shell", WorkspaceID: "source:live", CWD: &root})
		// Fault injection at the transport boundary, before the real server
		// sees its second split. The first split and ownership write succeed.
		realCLI := rexCLIPath()
		dir := t.TempDir()
		countFile, failureFile := filepath.Join(dir, "count"), filepath.Join(dir, "failed")
		launcher := "#!/bin/sh\ncase \"$*\" in\n*rex.session.new_split*)\n n=0; if [ -f " + shellQuote(countFile) + " ]; then read n < " + shellQuote(countFile) + "; fi\n n=$((n+1)); printf '%s\\n' \"$n\" > " + shellQuote(countFile) + "\n if [ \"$n\" = 2 ] && [ ! -f " + shellQuote(failureFile) + " ]; then printf failed > " + shellQuote(failureFile) + "; printf 'injected second split failure\\n' >&2; exit 1; fi\n;;\nesac\nexec " + shellQuote(realCLI) + " \"$@\"\n"
		if err := os.WriteFile(filepath.Join(dir, "rex"), []byte(launcher), 0700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	mapping := RexImportMap{}
	t.Cleanup(func() {
		// Import persists ownership before later operations. Clean up by those
		// exact IDs even when a Lua operation failed midway through the test.
		saved, err := LoadRexImportMap()
		if err != nil {
			t.Errorf("read disposable import ownership: %v", err)
			return
		}
		for _, id := range saved.Workspaces {
			if _, err := runRex("kill", id); err != nil {
				t.Errorf("close disposable import %s: %v", id, err)
			}
		}
	})
	if err := ApplyRexImport(source, &mapping); err != nil {
		if !injectFailure {
			t.Fatal(err)
		}
		if mapping.Panes["source:p2"] == "" || mapping.Panes["source:p4"] != "" {
			t.Fatalf("first split ownership was not retained: %+v", mapping)
		}
		if err := ApplyRexImport(source, &mapping); err != nil {
			t.Fatalf("resume after injected second split failure: %v", err)
		}
	} else if injectFailure {
		t.Fatal("split failure was not injected")
	}
	state, err := ReadRexState()
	if err != nil {
		t.Fatal(err)
	}
	session := rexSessionByID(state, mapping.Workspaces["source:live"])
	if session == nil || len(session.Windows) != 2 || len(mapping.Panes) != expectedPanes {
		t.Fatalf("live import structure: session=%+v mapping=%+v", session, mapping)
	}
	for _, pane := range source.Panes {
		id := mapping.Panes[pane.PaneID]
		if id == "" {
			t.Fatalf("pane %s was not imported", pane.PaneID)
		}
	}
	if err := ApplyRexImport(source, &mapping); err != nil {
		t.Fatalf("live import retry: %v", err)
	}
	state, err = ReadRexState()
	if err != nil {
		t.Fatal(err)
	}
	session = rexSessionByID(state, mapping.Workspaces["source:live"])
	blocks := 0
	for _, w := range session.Windows {
		blocks += len(w.Blocks)
	}
	if len(session.Windows) != 2 || blocks != expectedPanes {
		t.Fatalf("retry duplicated terminals: %d windows, %d blocks", len(session.Windows), blocks)
	}
}
