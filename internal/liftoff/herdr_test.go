package liftoff

import (
	"strings"
	"testing"
	"time"
)

func TestDecodeHerdrState(t *testing.T) {
	direct := `{"workspaces":[{"workspace_id":"w1","label":"feature-a"}],"tabs":[],"panes":[]}`
	enveloped := `{"id":"cli:api:snapshot","result":{"snapshot":{"workspaces":[{"workspace_id":"w1","label":"feature-a"}],"tabs":[],"panes":[]},"type":"session_snapshot"}}`

	for name, input := range map[string]string{
		"direct":    direct,
		"enveloped": enveloped,
	} {
		t.Run(name, func(t *testing.T) {
			state, err := decodeHerdrState([]byte(input))
			if err != nil {
				t.Fatal(err)
			}
			if len(state.Workspaces) != 1 || state.Workspaces[0].WorkspaceID != "w1" {
				t.Fatalf("decoded state = %+v", state)
			}
		})
	}
}

func TestDecodeHerdrStateRejectsEnvelopeWithoutSnapshot(t *testing.T) {
	_, err := decodeHerdrState([]byte(`{"id":"cli:api:snapshot","result":{"type":"session_snapshot"}}`))
	if err == nil || !strings.Contains(err.Error(), "missing snapshot") {
		t.Fatalf("error = %v, want missing snapshot", err)
	}
}

func TestFindHerdrWorkspacePrefersSavedID(t *testing.T) {
	cwd := "/tmp/kit/feature-a"
	state := HerdrState{Workspaces: []HerdrWorkspace{
		{WorkspaceID: "w1", Label: "feature-a"},
		{WorkspaceID: "w2", Label: "other", Worktree: &HerdrWorktreeRef{CheckoutPath: cwd}},
	}}
	got := FindHerdrWorkspace(state, "w1", "feature-a", cwd)
	if got == nil || got.WorkspaceID != "w1" {
		t.Fatalf("saved Herdr ID should win, got %+v", got)
	}
}

func TestFindHerdrWorkspaceMatchesCheckoutAndPanePaths(t *testing.T) {
	checkout := "/tmp/kit/feature-a"
	paneCWD := "/tmp/kit/feature-b/backend"
	state := HerdrState{
		Workspaces: []HerdrWorkspace{
			{WorkspaceID: "w1", Label: "feature-a", Worktree: &HerdrWorktreeRef{CheckoutPath: checkout}},
			{WorkspaceID: "w2", Label: "feature-b"},
		},
		Panes: []HerdrPane{{WorkspaceID: "w2", CWD: &paneCWD}},
	}
	if got := FindHerdrWorkspace(state, "", "feature-a", checkout); got == nil || got.WorkspaceID != "w1" {
		t.Fatalf("checkout path should match w1, got %+v", got)
	}
	if got := FindHerdrWorkspace(state, "", "feature-b", "/tmp/kit/feature-b"); got == nil || got.WorkspaceID != "w2" {
		t.Fatalf("pane path should match w2, got %+v", got)
	}
}

func TestFindHerdrWorkspaceAcceptsLegacyLabelPrefix(t *testing.T) {
	state := HerdrState{Workspaces: []HerdrWorkspace{{WorkspaceID: "w1", Label: "🔍 feature-a"}}}
	got := FindHerdrWorkspace(state, "", "feature-a", "/tmp/feature-a")
	if got == nil || got.WorkspaceID != "w1" {
		t.Fatalf("legacy label should be recoverable, got %+v", got)
	}
}

func TestWorkspaceAgentSummary(t *testing.T) {
	state := HerdrState{Panes: []HerdrPane{
		{WorkspaceID: "w1", Agent: stringPtr("claude"), AgentStatus: "working"},
		{WorkspaceID: "w1", Agent: stringPtr("codex"), AgentStatus: "blocked"},
		{WorkspaceID: "w2", Agent: stringPtr("gemini"), AgentStatus: "idle"},
	}}
	got := state.WorkspaceAgentSummary("w1")
	if got != "claude working, codex blocked" {
		t.Fatalf("summary = %q", got)
	}
	if got := state.WorkspaceAgentSummary("w3"); got != "idle" {
		t.Fatalf("empty summary = %q, want idle", got)
	}
}

func TestBuiltinHerdrLayouts(t *testing.T) {
	layouts := BuiltinHerdrLayouts()
	if got := layouts["default"].Tabs; strings.Join(got, ",") != "shell,logs" {
		t.Fatalf("default layout = %v", got)
	}
	if got := layouts["detailed"].Tabs; len(got) != 5 {
		t.Fatalf("detailed layout has %d tabs, want 5", len(got))
	}
}

func TestHerdrShellTabLabelMatchesGhostty(t *testing.T) {
	t.Setenv("KIT_NO_EMOJI", "")
	name := "voice-agent"
	want := EmojiFor(name) + " " + name
	if got := HerdrShellTabLabel(name); got != want {
		t.Fatalf("Herdr shell label = %q, want %q", got, want)
	}

	t.Setenv("KIT_NO_EMOJI", "1")
	if got := HerdrShellTabLabel(name); got != name {
		t.Fatalf("emoji-disabled Herdr shell label = %q, want %q", got, name)
	}
}

func TestHerdrMetadataAndLayoutsRoundTrip(t *testing.T) {
	setStateDir(t)
	c, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	c.Worktrees["feature-a"] = WorktreeMeta{
		HerdrSpace:      "feature-a",
		HerdrID:         "w1",
		HerdrLayout:     "ai",
		LastOpened:      now,
		PreferredAgents: []string{"claude", "codex"},
	}
	c.Layouts["team"] = HerdrLayout{Tabs: []string{"shell", "claude", "logs"}}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	c2, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	meta := c2.Worktrees["feature-a"]
	if meta.HerdrSpace != "feature-a" || meta.HerdrID != "w1" || meta.HerdrLayout != "ai" || !meta.LastOpened.Equal(now) {
		t.Fatalf("Herdr metadata not preserved: %+v", meta)
	}
	if got := c2.Layouts["team"].Tabs; strings.Join(got, ",") != "shell,claude,logs" {
		t.Fatalf("custom layout not preserved: %v", got)
	}
}

func TestHerdrTabCommandUsesKitAndPersistentAgents(t *testing.T) {
	logs := herdrTabCommand("feature-a", "/tmp/feature-a", "logs")
	if !strings.Contains(logs, "feature-a") || !strings.Contains(logs, "--wait") {
		t.Fatalf("logs command = %q", logs)
	}
	claude := herdrTabCommand("feature-a", "/tmp/feature-a", "claude")
	if !strings.Contains(claude, "exec claude") || !strings.Contains(claude, "${SHELL:-/bin/sh}") {
		t.Fatalf("claude command should preserve a terminal when unavailable: %q", claude)
	}
}

func stringPtr(value string) *string { return &value }
