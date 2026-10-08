package liftoff

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseRexStateSnapshotAndLuaEmptyTable(t *testing.T) {
	input := `{"sessions":[{"session_id":"session:one","label":"feature-a","windows":[{"window_id":"window:one","label":"shell","active":true,"blocks":[{"block_id":"block:one","label":"","creator_name":"com.superlogical.terminal","flavor":"com.superlogical.terminal.shell","placement":"placed","status":"running"}]}]}]}`
	state, err := parseRexState(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Sessions) != 1 || state.Sessions[0].SessionID != "session:one" {
		t.Fatalf("sessions = %+v", state.Sessions)
	}
	if got := state.Sessions[0].Windows[0].Blocks[0]; got.BlockID != "block:one" || got.Status != "running" {
		t.Fatalf("block = %+v", got)
	}
	for _, empty := range []string{`{"sessions":{}}`, `{"sessions":[]}`, `{}`} {
		state, err := parseRexState(empty)
		if err != nil || len(state.Sessions) != 0 {
			t.Fatalf("parse empty %s = %+v, %v", empty, state, err)
		}
	}
	if _, err := parseRexState("not json"); err == nil {
		t.Fatal("expected malformed snapshot error")
	}
}

// TestFakeRexCLI is the stateful subprocess behind the fake executable used by
// the adapter integration tests below. It records every command and persists
// session/window state across invocations, like the real server boundary.
func TestFakeRexCLI(t *testing.T) {
	if os.Getenv("KIT_REX_TEST_HELPER") != "1" {
		return
	}
	args := os.Args
	for i, arg := range args {
		if arg == "--" {
			args = args[i+1:]
			break
		}
	}
	if len(args) > 0 && args[0] == "--autostart=false" {
		args = args[1:]
	}
	var state RexState
	stateData, err := os.ReadFile(os.Getenv("KIT_REX_TEST_STATE"))
	if err == nil {
		_ = json.Unmarshal(stateData, &state)
	}
	appendRexLog(t, strings.ReplaceAll(strings.Join(args, " "), "\n", `\n`))
	if len(args) >= 2 && args[0] == "-C" {
		args = args[2:]
	}
	save := func() { data, _ := json.Marshal(state); _ = os.WriteFile(os.Getenv("KIT_REX_TEST_STATE"), data, 0600) }
	switch args[0] {
	case "client":
		_, _ = os.Stdout.WriteString(`{"clients":[{"client_id":"client:test-app","info":{"kind":"app"}}]}`)
	case "do":
		var tabs struct {
			Name      string `json:"name"`
			SessionID string `json:"session_id"`
			Tabs      []struct {
				Label string `json:"label"`
			} `json:"tabs"`
		}
		for i := range args {
			if args[i] == "--args" && i+1 < len(args) {
				_ = json.Unmarshal([]byte(args[i+1]), &tabs)
			}
		}
		if tabs.Name != "" && len(tabs.Tabs) > 0 {
			if os.Getenv("KIT_REX_DELAY_CREATE") == "1" {
				time.Sleep(100 * time.Millisecond)
			}
			id := "session:test-" + strings.ReplaceAll(tabs.Name, "/", "-")
			session := RexSession{SessionID: id, Label: tabs.Name}
			for i, tab := range tabs.Tabs {
				if os.Getenv("KIT_REX_FAIL_TAB") == tab.Label {
					_, _ = os.Stderr.WriteString("injected window failure")
					os.Exit(1)
				}
				session.Windows = append(session.Windows, RexWindow{WindowID: "window:" + id + ":" + tab.Label, Label: tab.Label, Active: i == 0})
			}
			state.Sessions = append(state.Sessions, session)
			save()
			_, _ = os.Stdout.WriteString(`{"session_id":"` + id + `"}`)
			os.Exit(0)
		}
		if len(tabs.Tabs) > 0 {
			for _, tab := range tabs.Tabs {
				if os.Getenv("KIT_REX_FAIL_TAB") == tab.Label {
					_, _ = os.Stderr.WriteString("injected window failure")
					os.Exit(1)
				}
				for i := range state.Sessions {
					if state.Sessions[i].SessionID == tabs.SessionID {
						state.Sessions[i].Windows = append(state.Sessions[i].Windows, RexWindow{WindowID: "window:" + tabs.SessionID + ":" + tab.Label, Label: tab.Label})
					}
				}
				save()
			}
			_, _ = os.Stdout.WriteString("true\n")
			os.Exit(0)
		}
		data, _ := json.Marshal(state)
		_, _ = os.Stdout.Write(append(data, '\n'))
	case "new":
		if os.Getenv("KIT_REX_DELAY_CREATE") == "1" {
			time.Sleep(100 * time.Millisecond)
		}
		name, window := args[1], "shell"
		for i := range args {
			if args[i] == "--window" && i+1 < len(args) {
				window = args[i+1]
			}
		}
		id := "session:test-" + strings.ReplaceAll(name, "/", "-")
		state.Sessions = append(state.Sessions, RexSession{SessionID: id, Label: name, Windows: []RexWindow{{WindowID: "window:" + id, Label: window, Active: true}}})
		save()
		_, _ = os.Stdout.WriteString(`{"session_id":"` + id + `"}`)
	case "window":
		if len(args) < 3 || args[1] != "new" {
			os.Exit(2)
		}
		label := ""
		for i := range args {
			if args[i] == "-s" && i+2 < len(args) {
				label = args[i+2]
				break
			}
		}
		if label == "" {
			os.Exit(2)
		}
		if os.Getenv("KIT_REX_FAIL_TAB") == label {
			_, _ = os.Stderr.WriteString("injected window failure")
			os.Exit(1)
		}
		id := ""
		for i := range args {
			if args[i] == "-s" && i+1 < len(args) {
				id = args[i+1]
			}
		}
		for i := range state.Sessions {
			if state.Sessions[i].SessionID == id {
				state.Sessions[i].Windows = append(state.Sessions[i].Windows, RexWindow{WindowID: "window:" + id + ":" + label, Label: label})
				break
			}
		}
		save()
	case "kill":
		if len(args) < 2 {
			os.Exit(2)
		}
		for i := range state.Sessions {
			if state.Sessions[i].SessionID == args[1] {
				state.Sessions = append(state.Sessions[:i], state.Sessions[i+1:]...)
				break
			}
		}
		save()
	case "focus":
		// Assert the adapter requests the already-active window by ID.
	default:
		os.Exit(2)
	}
	os.Exit(0)
}

func appendRexLog(t *testing.T, line string) {
	t.Helper()
	file, err := os.OpenFile(os.Getenv("KIT_REX_TEST_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err == nil {
		_, _ = file.WriteString(line + "\n")
		_ = file.Close()
	}
}

type rexCLIStub struct{ dir, stateFile, logFile string }

func installFakeRex(t *testing.T, initial RexState) rexCLIStub {
	t.Helper()
	dir := t.TempDir()
	stateFile, logFile := filepath.Join(dir, "state.json"), filepath.Join(dir, "calls.log")
	data, err := json.Marshal(initial)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateFile, data, 0600); err != nil {
		t.Fatal(err)
	}
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cli := filepath.Join(dir, "rex")
	launcher := "#!/bin/sh\nexec '" + bin + "' -test.run=^TestFakeRexCLI$ -- \"$@\"\n"
	if err := os.WriteFile(cli, []byte(launcher), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("KIT_REX_TEST_HELPER", "1")
	t.Setenv("KIT_REX_TEST_STATE", stateFile)
	t.Setenv("KIT_REX_TEST_LOG", logFile)
	t.Cleanup(func() { t.Setenv("KIT_REX_TEST_HELPER", "") })
	return rexCLIStub{dir: dir, stateFile: stateFile, logFile: logFile}
}

func (f rexCLIStub) state(t *testing.T) RexState {
	t.Helper()
	data, err := os.ReadFile(f.stateFile)
	if err != nil {
		t.Fatal(err)
	}
	var state RexState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	return state
}

func (f rexCLIStub) calls(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(f.logFile)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func addRexWorktree(t *testing.T, name, path string) {
	t.Helper()
	if err := WithConfigLock(func(c *Config) error { c.Worktrees[name] = WorktreeMeta{Path: path}; return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestOpenRexCreatesKitCommandTabsAndReusesWithoutRemovingManualTabs(t *testing.T) {
	setStateDir(t)
	fake := installFakeRex(t, RexState{})
	addRexWorktree(t, "feature-a", t.TempDir())
	opened, err := OpenRex("feature-a", t.TempDir(), "default")
	if err != nil {
		t.Fatal(err)
	}
	if opened.SessionID != "session:test-feature-a" {
		t.Fatalf("session ID = %q", opened.SessionID)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Worktrees["feature-a"].RexID != opened.SessionID {
		t.Fatalf("mapping = %+v", cfg.Worktrees["feature-a"])
	}
	calls := fake.calls(t)
	if !containsRexCall(calls, `"label":"logs"`) || !containsRexCall(calls, " log 'feature-a' --wait") {
		t.Fatalf("calls did not launch Kit logs command: %q", calls)
	}
	for _, call := range calls {
		if strings.HasPrefix(call, "focus ") || strings.HasPrefix(call, "attach ") {
			t.Fatalf("OpenRex must not focus or attach: %q", calls)
		}
	}
	state := fake.state(t)
	state.Sessions[0].Windows = append(state.Sessions[0].Windows, RexWindow{WindowID: "window:manual", Label: "manual"})
	writeFakeState(t, fake, state)
	before := len(fake.calls(t))
	opened, err = OpenRex("feature-a", t.TempDir(), "default")
	if err != nil {
		t.Fatal(err)
	}
	if opened.SessionID != "session:test-feature-a" {
		t.Fatalf("reopened session = %+v", opened)
	}
	if len(fake.calls(t)) != before+1 {
		t.Fatalf("reopen should only inspect; calls = %q", fake.calls(t)[before:])
	}
	if got := fake.state(t).Sessions[0].Windows; len(got) != 3 || got[2].Label != "manual" {
		t.Fatalf("manual tab not preserved: %+v", got)
	}
}

func TestConcurrentRexOpensCreateOneOwnedSession(t *testing.T) {
	setStateDir(t)
	fake := installFakeRex(t, RexState{})
	path := t.TempDir()
	addRexWorktree(t, "concurrent", path)
	t.Setenv("KIT_REX_DELAY_CREATE", "1")
	start := make(chan struct{})
	errs := make(chan error, 3)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := OpenRex("concurrent", path, "default")
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	creates := 0
	for _, call := range fake.calls(t) {
		if strings.HasPrefix(call, "do ") && strings.Contains(call, `"name":"concurrent"`) {
			creates++
		}
	}
	if creates != 1 {
		t.Fatalf("concurrent opens created %d sessions; ownership metadata can track only one", creates)
	}
}

func TestOpenRexCreationIsAtomicAndRetryable(t *testing.T) {
	setStateDir(t)
	fake := installFakeRex(t, RexState{})
	addRexWorktree(t, "failure-a", t.TempDir())
	t.Setenv("KIT_REX_FAIL_TAB", "logs")
	if _, err := OpenRex("failure-a", t.TempDir(), "default"); err == nil {
		t.Fatal("expected injected tab failure")
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Worktrees["failure-a"].RexID != "" || len(fake.state(t).Sessions) != 0 {
		t.Fatalf("failed creation left a half-built session: mapping=%+v sessions=%+v", cfg.Worktrees["failure-a"], fake.state(t).Sessions)
	}
	t.Setenv("KIT_REX_FAIL_TAB", "")
	if _, err := OpenRex("failure-a", t.TempDir(), "default"); err != nil {
		t.Fatal(err)
	}
	if got := fake.state(t).Sessions; len(got) != 1 || len(got[0].Windows) < 2 {
		t.Fatalf("retry did not create the full layout: %+v", got)
	}
}

func TestOpenRexAddsMissingTabToExistingSession(t *testing.T) {
	setStateDir(t)
	fake := installFakeRex(t, RexState{})
	addRexWorktree(t, "missing-a", t.TempDir())
	if _, err := OpenRex("missing-a", t.TempDir(), "default"); err != nil {
		t.Fatal(err)
	}
	state := fake.state(t)
	state.Sessions[0].Windows = state.Sessions[0].Windows[:1]
	writeFakeState(t, fake, state)
	if _, err := OpenRex("missing-a", t.TempDir(), "default"); err != nil {
		t.Fatal(err)
	}
	if got := fake.state(t).Sessions[0].Windows; len(got) < 2 {
		t.Fatalf("missing tab not added: %+v", got)
	}
}

func TestOpenRexRejectsLabelCollisionWithoutAdoptingOrCreating(t *testing.T) {
	setStateDir(t)
	fake := installFakeRex(t, RexState{Sessions: []RexSession{{SessionID: "session:unrelated", Label: "collision"}}})
	addRexWorktree(t, "collision", t.TempDir())
	if _, err := OpenRex("collision", t.TempDir(), "default"); err == nil || !strings.Contains(err.Error(), "unrelated session") {
		t.Fatalf("collision error = %v", err)
	}
	if got := fake.state(t).Sessions[0].SessionID; got != "session:unrelated" {
		t.Fatalf("collision session mutated: %q", got)
	}
	if containsRexCall(fake.calls(t), "new collision") || containsRexCall(fake.calls(t), "kill") {
		t.Fatalf("destructive fallback occurred: %q", fake.calls(t))
	}
}

func TestOpenRexReusesImportedShellRoleAfterRename(t *testing.T) {
	setStateDir(t)
	fake := installFakeRex(t, RexState{Sessions: []RexSession{{
		SessionID: "session:imported", Label: "renamed-workspace",
		Windows: []RexWindow{
			{WindowID: "window:root", Label: "custom root"},
			{WindowID: "window:logs", Label: "logs"},
		},
	}}})
	path := t.TempDir()
	if err := WithConfigLock(func(c *Config) error {
		c.Worktrees["imported"] = WorktreeMeta{Path: path, RexID: "session:imported", RexShellWindow: "window:root"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenRex("imported", path, "default"); err != nil {
		t.Fatal(err)
	}
	if got := fake.state(t).Sessions[0].Windows; len(got) != 2 || got[0].Label != "custom root" {
		t.Fatalf("reopening imported layout duplicated or renamed shell: %+v", got)
	}
}

func TestOpenRexRejectsMalformedConfigBeforeCLIMutation(t *testing.T) {
	dir := setStateDir(t)
	fake := installFakeRex(t, RexState{})
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("[broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenRex("broken", t.TempDir(), "default"); err == nil {
		t.Fatal("expected malformed config error")
	}
	if len(fake.calls(t)) != 0 {
		t.Fatalf("CLI called despite malformed config: %q", fake.calls(t))
	}
}

func TestCloseRexKillsOnlySavedIDAndDoesNotResurrectMissingRecord(t *testing.T) {
	setStateDir(t)
	fake := installFakeRex(t, RexState{Sessions: []RexSession{{SessionID: "session:mapped", Label: "same"}, {SessionID: "session:other", Label: "same"}}})
	addRexWorktree(t, "same", t.TempDir())
	if err := WithConfigLock(func(c *Config) error {
		m := c.Worktrees["same"]
		m.RexID = "session:mapped"
		c.Worktrees["same"] = m
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := CloseRex("same", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if got := fake.state(t).Sessions; len(got) != 1 || got[0].SessionID != "session:other" {
		t.Fatalf("closed wrong session: %+v", got)
	}
	for _, call := range fake.calls(t) {
		if strings.HasPrefix(call, "kill ") && strings.TrimSuffix(call, " --autostart=false") != "kill session:mapped" {
			t.Fatalf("unexpected kill: %q", call)
		}
	}
	if err := WithConfigLock(func(c *Config) error { delete(c.Worktrees, "missing"); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := CloseRex("missing", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Worktrees["missing"]; ok {
		t.Fatal("CloseRex resurrected absent worktree record")
	}
}

func TestCloseRexWithoutMappingNeverUsesLabelFallback(t *testing.T) {
	setStateDir(t)
	fake := installFakeRex(t, RexState{Sessions: []RexSession{{SessionID: "session:unrelated", Label: "orphan"}}})
	addRexWorktree(t, "orphan", t.TempDir())
	if err := CloseRex("orphan", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if len(fake.state(t).Sessions) != 1 || containsRexCall(fake.calls(t), "kill") {
		t.Fatalf("unmapped session was touched: %+v, %q", fake.state(t), fake.calls(t))
	}
}

func TestCloseRexCleansImportedSecondarySessionsByCheckout(t *testing.T) {
	setStateDir(t)
	path := t.TempDir()
	fake := installFakeRex(t, RexState{Sessions: []RexSession{
		{SessionID: "session:primary", Label: "same"},
		{SessionID: "session:secondary", Label: "another"},
		{SessionID: "session:unrelated", Label: "same"},
	}})
	if err := WithConfigLock(func(c *Config) error {
		c.Worktrees["primary"] = WorktreeMeta{Path: path, RexID: "session:primary"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := SaveRexImportMap(RexImportMap{
		Workspaces: map[string]string{"w1": "session:primary", "w2": "session:secondary"},
		Checkouts:  map[string]string{"w1": path, "w2": path},
	}); err != nil {
		t.Fatal(err)
	}
	if err := CloseRex("primary", path); err != nil {
		t.Fatal(err)
	}
	if sessions := fake.state(t).Sessions; len(sessions) != 1 || sessions[0].SessionID != "session:unrelated" {
		t.Fatalf("cleanup leaked imported session or touched unrelated one: %+v", sessions)
	}
}

func TestFocusRexClientUsesExistingActiveWindow(t *testing.T) {
	setStateDir(t)
	fake := installFakeRex(t, RexState{Sessions: []RexSession{{SessionID: "session:focus", Label: "focus", Windows: []RexWindow{{WindowID: "window:active", Label: "shell", Active: true}}}}})
	if err := FocusRexClient("session:focus"); err != nil {
		t.Fatal(err)
	}
	if !containsRexCall(fake.calls(t), "-C client:test-app do session.select session_id=session:focus window_id=window:active") {
		t.Fatalf("focus command = %q", fake.calls(t))
	}
}

func TestReadRexStateUsesLuaSnapshotThroughCLI(t *testing.T) {
	fake := installFakeRex(t, RexState{})
	state, err := ReadRexState()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Sessions) != 0 {
		t.Fatalf("empty snapshot = %+v", state)
	}
	if !strings.Contains(strings.Join(fake.calls(t), "\n"), "do -e") || !strings.Contains(rexSnapshotLua, "session.list") {
		t.Fatalf("no Lua snapshot request: %q", fake.calls(t))
	}
}

func TestOpenRexRejectsUnknownLayoutBeforeCLI(t *testing.T) {
	setStateDir(t)
	fake := installFakeRex(t, RexState{})
	addRexWorktree(t, "bad-layout", t.TempDir())
	if _, err := OpenRex("bad-layout", t.TempDir(), "unknown"); err == nil {
		t.Fatal("expected unknown layout error")
	}
	if len(fake.calls(t)) != 0 {
		t.Fatalf("CLI called for invalid layout: %q", fake.calls(t))
	}
}

func TestOpenRexMasterUsesDedicatedConfigID(t *testing.T) {
	setStateDir(t)
	fake := installFakeRex(t, RexState{})
	session, err := OpenRex("master", t.TempDir(), "default")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Settings.RexMasterSession != session.SessionID {
		t.Fatalf("master Rex ID = %q, want %q", cfg.Settings.RexMasterSession, session.SessionID)
	}
	if _, ok := cfg.Worktrees["master"]; ok {
		t.Fatal("master session must not create a worktree record")
	}
	if err := CloseRex("master", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if len(fake.state(t).Sessions) != 0 {
		t.Fatal("explicit mapped master close did not close its Rex session")
	}
	cfg, err = LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Worktrees["master"]; ok {
		t.Fatal("master close resurrected a worktree record")
	}
}

func TestRexDisposableLiveIntegration(t *testing.T) {
	if os.Getenv("KIT_TEST_REX") != "1" {
		t.Skip("set KIT_TEST_REX=1 to run isolated Rex integration test")
	}
	if !RexAvailable() {
		t.Fatal("KIT_TEST_REX=1 but Rex CLI is unavailable")
	}
	setStateDir(t)
	path := t.TempDir()
	name := "kit-rex-test-" + filepath.Base(path)
	addRexWorktree(t, name, path)
	if err := WithConfigLock(func(c *Config) error {
		// A Go test executable is not the Kit CLI. Never launch it as a log
		// viewer (it would run its test suite in another terminal).
		c.Layouts["live-test"] = WorkspaceLayout{Tabs: []string{"shell"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cfg, err := LoadConfig()
		if err != nil {
			t.Errorf("read disposable session ownership: %v", err)
			return
		}
		if id := cfg.Worktrees[name].RexID; id != "" {
			if _, err := runRex("kill", id); err != nil {
				t.Errorf("close disposable Rex session %s: %v", id, err)
			}
		}
	})
	session, err := OpenRex(name, path, "live-test")
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenRex(name, path, "live-test")
	if err != nil || reopened.SessionID != session.SessionID || len(reopened.Windows) != 1 {
		t.Fatalf("live reopen duplicated state: %+v, %v", reopened, err)
	}
	if _, err := runRex("split", "-s", session.SessionID, "--cwd", path, "--json"); err != nil {
		t.Fatal(err)
	}
	state, err := ReadRexState()
	if err != nil {
		t.Fatal(err)
	}
	if findRexSession(state, session.SessionID, "") == nil {
		t.Fatalf("created session %s missing from Rex state", session.SessionID)
	}
	if err := CloseRex(name, path); err != nil {
		t.Fatal(err)
	}
	state, err = ReadRexState()
	if err != nil || findRexSession(state, session.SessionID, "") != nil {
		t.Fatalf("live close did not remove test session: %v", err)
	}
}

func writeFakeState(t *testing.T, f rexCLIStub, state RexState) {
	t.Helper()
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.stateFile, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func containsRexCall(calls []string, fragment string) bool {
	for _, call := range calls {
		if strings.Contains(call, fragment) {
			return true
		}
	}
	return false
}

func TestCloseOwnRexTabNeedsRexTerminalEnv(t *testing.T) {
	fake := installFakeRex(t, RexState{})
	t.Setenv("REX_SESSION", "")
	t.Setenv("REX_BLOCK", "")
	CloseOwnRexTab()
	if len(fake.calls(t)) != 0 {
		t.Fatalf("outside Rex must not call the CLI: %q", fake.calls(t))
	}
	t.Setenv("REX_SESSION", "session:wizard")
	t.Setenv("REX_BLOCK", "block:wizard")
	CloseOwnRexTab()
	calls := strings.Join(fake.calls(t), "\n")
	if !strings.Contains(calls, "do -s session:wizard") || !strings.Contains(calls, `"block_id":"block:wizard"`) {
		t.Fatalf("close request = %q", calls)
	}
}

func TestRexOpenWorkspacesMatchesSavedSessionIDs(t *testing.T) {
	setStateDir(t)
	installFakeRex(t, RexState{Sessions: []RexSession{{SessionID: "session:live", Label: "a"}}})
	if err := WithConfigLock(func(c *Config) error {
		c.Worktrees["open-wt"] = WorktreeMeta{RexID: "session:live"}
		c.Worktrees["stale-wt"] = WorktreeMeta{RexID: "session:gone"}
		c.Worktrees["none-wt"] = WorktreeMeta{}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	open, err := RexOpenWorkspaces()
	if err != nil {
		t.Fatal(err)
	}
	if !open["open-wt"] || open["stale-wt"] || open["none-wt"] {
		t.Fatalf("open = %v", open)
	}
}
