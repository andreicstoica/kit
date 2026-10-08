package liftoff

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRexSimpleLayoutDisposableLive(t *testing.T) {
	if os.Getenv("KIT_TEST_REX") != "1" {
		t.Skip("set KIT_TEST_REX=1")
	}
	path := t.TempDir()
	args, _ := json.Marshal(map[string]string{"name": "kit-simple-test-" + filepath.Base(path), "cwd": path, "command": "printf '\\033]7501;state=done:app=kit-test\\033\\\\'; sleep 30"})
	out, err := runRex("do", "-e", rexSimpleSessionLua, "--args", string(args))
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		SessionID      string `json:"session_id"`
		InitialWindows []struct {
			BlockIDs []string `json:"block_ids"`
		} `json:"initial_windows"`
	}
	if err := json.Unmarshal([]byte(out), &created); err != nil || created.SessionID == "" {
		t.Fatalf("create response: %s, %v", out, err)
	}
	t.Cleanup(func() {
		if _, err := runRex("kill", created.SessionID); err != nil {
			t.Error(err)
		}
	})
	state, err := ReadRexState()
	if err != nil {
		t.Fatal(err)
	}
	session := findRexSession(state, created.SessionID, "")
	if session == nil || len(session.Windows) != 1 || len(session.Windows[0].Blocks) != 2 {
		t.Fatalf("expected one tab with shell and agent panes: %+v", session)
	}
	if len(created.InitialWindows) != 1 || len(created.InitialWindows[0].BlockIDs) != 2 {
		t.Fatalf("missing published pane identities: %s", out)
	}
	view, err := runRex("do", "-s", created.SessionID, "-e", "return rex.session.view{}")
	if err != nil {
		t.Fatal(err)
	}
	var focused struct {
		Window struct {
			BlockID string `json:"focused_block_id"`
		} `json:"focused_window"`
	}
	if err := json.Unmarshal([]byte(view), &focused); err != nil {
		t.Fatal(err)
	}
	if focused.Window.BlockID != created.InitialWindows[0].BlockIDs[1] {
		t.Fatalf("new workspace focused shell, not Claude pane: %s", view)
	}
}
