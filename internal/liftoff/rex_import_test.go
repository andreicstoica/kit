package liftoff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanRexImportSafeReconstructionAndRetryCases(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	missing := filepath.Join(root, "gone")
	unknown := "mystery-agent"
	claude := "claude"
	validCWD := filepath.Join(root, "nested")
	if err := os.Mkdir(validCWD, 0o755); err != nil {
		t.Fatal(err)
	}
	state := HerdrState{
		Workspaces: []HerdrWorkspace{
			{WorkspaceID: "source-1", Label: "feature", Worktree: &HerdrWorktreeRef{CheckoutPath: root}},
			{WorkspaceID: "source-2", Label: "gone", Worktree: &HerdrWorktreeRef{CheckoutPath: missing}},
			{WorkspaceID: "source-3", Label: "collision", Worktree: &HerdrWorktreeRef{CheckoutPath: root}},
		},
		Tabs: []HerdrTab{{TabID: "tab-1", WorkspaceID: "source-1", Label: "agent tab"}},
		Panes: []HerdrPane{
			{PaneID: "pane-1", TabID: "tab-1", CWD: &missing, Agent: &claude},
			{PaneID: "pane-2", TabID: "tab-1", CWD: &outside, Agent: &unknown},
		},
	}
	items := PlanRexImport(state, RexImportMap{Workspaces: map[string]string{"source-1": "rex-1"}}, map[string]bool{"rex-1": true, "source-3": true})
	if len(items) != 3 {
		t.Fatalf("got %d items", len(items))
	}
	if !strings.Contains(items[0].Skip, "mapped") {
		t.Fatalf("retry not recognized: %#v", items[0])
	}
	if len(items[0].Tabs) != 1 || len(items[0].Tabs[0].Panes) != 2 {
		t.Fatal("tab/pane tree not retained")
	}
	if !items[0].Tabs[0].Panes[0].KnownAgent || items[0].Tabs[0].Panes[0].CWD != root {
		t.Fatal("stale pane cwd should fall back to checkout; known agent resumable")
	}
	if items[0].Tabs[0].Panes[1].KnownAgent || items[0].Tabs[0].Panes[1].Agent != unknown {
		t.Fatal("unknown agent must not be auto-launched")
	}
	if items[1].Skip != "" || len(items[1].Warnings) == 0 {
		t.Fatal("stale checkout must be surfaced, not silently discarded")
	}
	if items[2].Skip == "" {
		t.Fatal("unmapped source ID collision must be protected")
	}
	if len(items[0].Warnings) == 0 {
		t.Fatal("outside cwd and unknown agent should be warned")
	}
}

func TestRexImportRefusalIsIndependentOfDisplayCopy(t *testing.T) {
	root := t.TempDir()
	source := HerdrState{Workspaces: []HerdrWorkspace{
		{WorkspaceID: "retry", Label: "retry", Worktree: &HerdrWorktreeRef{CheckoutPath: root}},
		{WorkspaceID: "missing", Label: "missing", Worktree: &HerdrWorktreeRef{CheckoutPath: root}},
		{WorkspaceID: "id-collision", Label: "id", Worktree: &HerdrWorktreeRef{CheckoutPath: root}},
		{WorkspaceID: "label-collision", Label: "taken", Worktree: &HerdrWorktreeRef{CheckoutPath: root}},
		{WorkspaceID: "duplicate-1", Label: "duplicate", Worktree: &HerdrWorktreeRef{CheckoutPath: root}},
		{WorkspaceID: "duplicate-2", Label: "duplicate", Worktree: &HerdrWorktreeRef{CheckoutPath: root}},
	}}
	mapping := RexImportMap{Workspaces: map[string]string{"retry": "owned", "missing": "gone"}}
	items := PlanRexImport(source, mapping, map[string]bool{"owned": true, "id-collision": true})
	AnnotateRexImportCollisions(items, RexState{Sessions: []RexSession{{SessionID: "unrelated", Label: "taken"}}}, mapping)
	for i := range items {
		items[i].Skip = "Copy can be edited without changing the safety contract"
		if got, want := items[i].Blocked, items[i].SourceID != "retry"; got != want {
			t.Fatalf("refusal for %s = %v, want %v", items[i].SourceID, got, want)
		}
	}
}

func TestPlanRexImportDoesNotReadPaneCommands(t *testing.T) {
	root := t.TempDir()
	state := HerdrState{Workspaces: []HerdrWorkspace{{WorkspaceID: "w", Worktree: &HerdrWorktreeRef{CheckoutPath: root}}}}
	items := PlanRexImport(state, RexImportMap{}, nil)
	if len(items) != 1 || len(items[0].Tabs) != 0 {
		t.Fatalf("unexpected plan: %#v", items)
	}
}

func TestPlanRexImportFallsBackToPaneCWDAndWarnsOnLabelCollision(t *testing.T) {
	root := t.TempDir()
	state := HerdrState{
		Workspaces: []HerdrWorkspace{{WorkspaceID: "herdr-x", Label: "shared-name"}},
		Tabs:       []HerdrTab{{TabID: "tab-x", WorkspaceID: "herdr-x", Label: "shell"}},
		Panes:      []HerdrPane{{PaneID: "pane-x", TabID: "tab-x", WorkspaceID: "herdr-x", CWD: &root}},
	}
	items := PlanRexImport(state, RexImportMap{}, nil)
	if len(items) != 1 || items[0].CWD != root {
		t.Fatalf("pane cwd fallback = %#v", items)
	}
	AnnotateRexImportCollisions(items, RexState{Sessions: []RexSession{{SessionID: "unrelated", Label: "shared-name"}}}, RexImportMap{})
	if !strings.Contains(items[0].Skip, "same-label") {
		t.Fatalf("collision missing from dry-run plan: %#v", items[0])
	}
}

func TestRexInitialShellUsesFirstPaneCWD(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	cwd := nested
	ws := HerdrWorkspace{WorkspaceID: "w", Worktree: &HerdrWorktreeRef{CheckoutPath: root}}
	state := HerdrState{Panes: []HerdrPane{{WorkspaceID: "w", CWD: &cwd}}}
	if got := workspaceInitialCWD(state, ws); got != nested {
		t.Fatalf("initial Rex shell cwd = %q, want nested Herdr pane cwd %q", got, nested)
	}
}
