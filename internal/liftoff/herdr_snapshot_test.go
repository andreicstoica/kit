package liftoff

import "testing"

// Captured from `herdr api snapshot` on herdr 0.7.5, trimmed to the fields Kit
// reads. The payload lives under result.snapshot, not at the top level.
const envelopedSnapshot = `{
  "id": "cli:api:snapshot",
  "result": {
    "type": "snapshot",
    "snapshot": {
      "protocol": 1,
      "focused_workspace_id": "wC",
      "workspaces": [
        {"workspace_id":"wC","label":"master","pane_count":2,"tab_count":1,"agent_status":"working"},
        {"workspace_id":"wG","label":"google-lite-sync-revamp","pane_count":3,"tab_count":2,"agent_status":"idle"}
      ],
      "tabs": [
        {"tab_id":"wC:tA","workspace_id":"wC","label":"shell"}
      ],
      "panes": [
        {"pane_id":"wC:pH","tab_id":"wC:tA","workspace_id":"wC","cwd":"/Users/acs/liftoff/liftoff-app-master"}
      ]
    }
  }
}`

// The pre-envelope shape older Herdr builds emitted.
const flatSnapshot = `{
  "workspaces": [{"workspace_id":"wC","label":"master","pane_count":2,"tab_count":1}],
  "tabs": [{"tab_id":"wC:tA","workspace_id":"wC","label":"shell"}],
  "panes": [{"pane_id":"wC:pH","tab_id":"wC:tA","workspace_id":"wC"}]
}`

func TestParseHerdrSnapshotEnveloped(t *testing.T) {
	state, err := parseHerdrSnapshot(envelopedSnapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(state.Workspaces) != 2 {
		t.Fatalf("workspaces = %d, want 2", len(state.Workspaces))
	}
	if state.Workspaces[0].WorkspaceID != "wC" || state.Workspaces[0].Label != "master" {
		t.Fatalf("first workspace = %+v", state.Workspaces[0])
	}
	if len(state.Tabs) != 1 || len(state.Panes) != 1 {
		t.Fatalf("tabs = %d, panes = %d, want 1 and 1", len(state.Tabs), len(state.Panes))
	}
}

func TestParseHerdrSnapshotFlatLegacy(t *testing.T) {
	state, err := parseHerdrSnapshot(flatSnapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(state.Workspaces) != 1 || state.Workspaces[0].Label != "master" {
		t.Fatalf("workspaces = %+v", state.Workspaces)
	}
}

// The regression this fixes: an enveloped response must never decode to an
// empty state, because callers read "no workspaces" as "it does not exist".
func TestParseHerdrSnapshotEnvelopedIsNeverSilentlyEmpty(t *testing.T) {
	state, err := parseHerdrSnapshot(envelopedSnapshot)
	if err == nil && len(state.Workspaces) == 0 {
		t.Fatal("enveloped snapshot decoded to an empty state without an error")
	}
}

func TestParseHerdrSnapshotEnvelopeWithoutPayloadErrors(t *testing.T) {
	if _, err := parseHerdrSnapshot(`{"id":"cli:api:snapshot","result":{"type":"snapshot"}}`); err == nil {
		t.Fatal("expected an error when result carries no snapshot")
	}
}

func TestParseHerdrSnapshotRejectsGarbage(t *testing.T) {
	if _, err := parseHerdrSnapshot("not json"); err == nil {
		t.Fatal("expected a parse error")
	}
}

// End-to-end through the matcher that OpenHerdr actually calls: with the fix
// in place, an existing "master" workspace is found instead of re-created.
func TestFindHerdrWorkspaceLocatesLabelFromEnvelopedSnapshot(t *testing.T) {
	state, err := parseHerdrSnapshot(envelopedSnapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := FindHerdrWorkspace(state, "", "master", "/Users/acs/liftoff/liftoff-app-master")
	if got == nil {
		t.Fatal("FindHerdrWorkspace returned nil for an existing workspace")
	}
	if got.WorkspaceID != "wC" {
		t.Fatalf("workspace id = %q, want wC", got.WorkspaceID)
	}
}
