package liftoff

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const testAgentBrief = "Fix the login redirect; secret-brief-marker"

func testAgentArgv() []string {
	return []string{"/opt/test/bin/claude", "--model", "opus", testAgentBrief}
}

// agentWorktree creates a Kit-mapped feature worktree under a temporary root.
func agentWorktree(t *testing.T, name string, meta WorktreeMeta) (Layout, string) {
	t.Helper()
	layout := newMasterRepo(t)
	layout.Root = filepath.Dir(layout.Master)
	path := layout.WorktreePath(name)
	runGit(t, layout.Master, "worktree", "add", "-b", name, path)
	meta.Path = path
	if err := WithConfigLock(func(c *Config) error { c.Worktrees[name] = meta; return nil }); err != nil {
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return layout, real
}

func installAgentFakeRex(t *testing.T, state RexState) rexCLIStub {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	fake := installFakeRex(t, state)
	if err := os.Symlink(git, filepath.Join(fake.dir, "git")); err != nil {
		t.Fatal(err)
	}
	return fake
}

func agentLaunchCall(t *testing.T, calls []string) map[string]any {
	t.Helper()
	for _, call := range calls {
		if !strings.Contains(call, `"argv"`) {
			continue
		}
		i, j := strings.Index(call, "--args "), strings.LastIndex(call, "}")
		var payload map[string]any
		if err := json.Unmarshal([]byte(call[i+len("--args "):j+1]), &payload); err != nil {
			t.Fatalf("parse launch payload %q: %v", call, err)
		}
		return payload
	}
	t.Fatalf("no agent launch call in %q", calls)
	return nil
}

func TestLaunchRexAgentReusesLiveSessionWithoutAddingLayoutTabs(t *testing.T) {
	setStateDir(t)
	layout, path := agentWorktree(t, "feature-a", WorktreeMeta{RexID: "session:live", RexLayout: "ai", RexShellWindow: "window:shell"})
	fake := installAgentFakeRex(t, RexState{Sessions: []RexSession{{
		SessionID: "session:live", Label: "feature-a",
		Windows: []RexWindow{{WindowID: "window:shell", Active: true}},
	}}})

	result, err := LaunchRexAgent(layout, "feature-a", testAgentArgv())
	if err != nil {
		t.Fatal(err)
	}
	if result.FocusErr != nil {
		t.Fatalf("focus: %v", result.FocusErr)
	}
	if result.SessionID != "session:live" || result.WindowID != "window:session:live:agent" || result.Path != path {
		t.Fatalf("result = %+v", result)
	}
	calls := fake.calls(t)
	if containsRexCall(calls, `"tabs"`) {
		t.Fatalf("launch must not add layout tabs (the ai layout would start another agent): %q", calls)
	}
	payload := agentLaunchCall(t, calls)
	argv, _ := json.Marshal(payload["argv"])
	if string(argv) != `["/opt/test/bin/claude","--model","opus","`+testAgentBrief+`"]` {
		t.Fatalf("argv = %s", argv)
	}
	if payload["cwd"] != path || payload["label"] != "agent" || payload["session_id"] != "session:live" {
		t.Fatalf("payload = %v", payload)
	}
	if !containsRexCall(calls, "-C client:test-app do session.select session_id=session:live window_id=window:session:live:agent") {
		t.Fatalf("new window was not selected in the app: %q", calls)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if m := cfg.Worktrees["feature-a"]; m.RexID != "session:live" || m.RexLayout != "ai" {
		t.Fatalf("mapping changed: %+v", m)
	}
}

func TestLaunchRexAgentCreatesShellOnlyDefaultSession(t *testing.T) {
	setStateDir(t)
	layout, _ := agentWorktree(t, "feature-b", WorktreeMeta{})
	fake := installAgentFakeRex(t, RexState{})

	result, err := LaunchRexAgent(layout, "feature-b", testAgentArgv())
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionID != "session:test-feature-b" || result.FocusErr != nil {
		t.Fatalf("result = %+v", result)
	}
	calls := fake.calls(t)
	if containsRexCall(calls, `"label":"claude"`) || containsRexCall(calls, "exec claude") {
		t.Fatalf("session creation must not start a layout agent: %q", calls)
	}
	var labels []string
	for _, w := range fake.state(t).Sessions[0].Windows {
		labels = append(labels, w.Label)
	}
	if strings.Join(labels, ",") != ",logs,agent" {
		t.Fatalf("windows = %q", labels)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if m := cfg.Worktrees["feature-b"]; m.RexID != result.SessionID || m.RexLayout != "default" {
		t.Fatalf("mapping = %+v", m)
	}
}

func TestLaunchRexAgentFocusFailureStillReportsLaunch(t *testing.T) {
	for _, env := range []string{"KIT_REX_NO_APP", "KIT_REX_FAIL_SELECT"} {
		t.Run(env, func(t *testing.T) {
			setStateDir(t)
			layout, _ := agentWorktree(t, "feature-c", WorktreeMeta{})
			installAgentFakeRex(t, RexState{})
			t.Setenv(env, "1")
			result, err := LaunchRexAgent(layout, "feature-c", testAgentArgv())
			if err != nil {
				t.Fatalf("a launched agent must not be reported as a failure: %v", err)
			}
			if result.WindowID == "" || result.FocusErr == nil {
				t.Fatalf("result = %+v", result)
			}
		})
	}
}

func TestLaunchRexAgentDoesNotGuessWindowAfterAmbiguousRexResponse(t *testing.T) {
	for _, mode := range []string{"timeout", "no-id"} {
		t.Run(mode, func(t *testing.T) {
			setStateDir(t)
			layout, _ := agentWorktree(t, "feature-d", WorktreeMeta{})
			installAgentFakeRex(t, RexState{})
			t.Setenv("KIT_REX_AGENT_RESULT", mode)
			result, err := LaunchRexAgent(layout, "feature-d", testAgentArgv())
			var unconfirmed *RexAgentUnconfirmedError
			if !errors.As(err, &unconfirmed) || result.WindowID != "" {
				t.Fatalf("result = %+v, err = %v", result, err)
			}
		})
	}
}

func TestLaunchRexAgentReportsUnconfirmedLaunchDistinctly(t *testing.T) {
	setStateDir(t)
	layout, _ := agentWorktree(t, "feature-g", WorktreeMeta{})
	installAgentFakeRex(t, RexState{})
	t.Setenv("KIT_REX_AGENT_RESULT", "timeout-ambiguous")
	_, err := LaunchRexAgent(layout, "feature-g", testAgentArgv())
	var unconfirmed *RexAgentUnconfirmedError
	if !errors.As(err, &unconfirmed) || unconfirmed.SessionID != "session:test-feature-g" {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "secret-brief-marker") {
		t.Fatalf("error leaks the brief: %v", err)
	}
}

func TestLaunchRexAgentAcceptsAdoptedNonKebabName(t *testing.T) {
	setStateDir(t)
	layout, _ := agentWorktree(t, "Login_Fix", WorktreeMeta{Adopted: true})
	installAgentFakeRex(t, RexState{})
	if _, err := LaunchRexAgent(layout, "Login_Fix", testAgentArgv()); err != nil {
		t.Fatal(err)
	}
}

func TestLaunchRexAgentFailureWithholdsBrief(t *testing.T) {
	setStateDir(t)
	layout, _ := agentWorktree(t, "feature-e", WorktreeMeta{})
	installAgentFakeRex(t, RexState{})
	t.Setenv("KIT_REX_FAIL_AGENT", "1")
	_, err := LaunchRexAgent(layout, "feature-e", testAgentArgv())
	if err == nil {
		t.Fatal("expected launch failure")
	}
	if strings.Contains(err.Error(), "secret-brief-marker") {
		t.Fatalf("error leaks the brief: %v", err)
	}
}

func TestLaunchRexAgentRejectsUnsafeDestinationsBeforeRex(t *testing.T) {
	cases := map[string]func(t *testing.T) (Layout, string){
		"unknown": func(t *testing.T) (Layout, string) {
			layout, _ := agentWorktree(t, "known", WorktreeMeta{})
			return layout, "other"
		},
		"master": func(t *testing.T) (Layout, string) {
			layout, _ := agentWorktree(t, "known", WorktreeMeta{})
			return layout, "master"
		},
		"traversal": func(t *testing.T) (Layout, string) {
			layout, _ := agentWorktree(t, "known", WorktreeMeta{})
			return layout, "../known"
		},
		"parked": func(t *testing.T) (Layout, string) {
			layout, _ := agentWorktree(t, "parked", WorktreeMeta{Parked: true})
			return layout, "parked"
		},
		"cleanup pending": func(t *testing.T) (Layout, string) {
			layout, _ := agentWorktree(t, "doomed", WorktreeMeta{CleanupPending: true})
			return layout, "doomed"
		},
		"cleanup intent": func(t *testing.T) (Layout, string) {
			layout, _ := agentWorktree(t, "doomed", WorktreeMeta{Cleanup: &CleanupIntent{}})
			return layout, "doomed"
		},
		"not a git worktree": func(t *testing.T) (Layout, string) {
			layout, path := agentWorktree(t, "plain", WorktreeMeta{})
			if err := os.Remove(filepath.Join(path, ".git")); err != nil {
				t.Fatal(err)
			}
			return layout, "plain"
		},
		"replacement repository": func(t *testing.T) (Layout, string) {
			layout, path := agentWorktree(t, "replaced", WorktreeMeta{})
			if err := os.Remove(filepath.Join(path, ".git")); err != nil {
				t.Fatal(err)
			}
			runGit(t, path, "init", "-b", "replaced")
			return layout, "replaced"
		},
		"symlinked path": func(t *testing.T) (Layout, string) {
			layout, _ := agentWorktree(t, "linked", WorktreeMeta{})
			canonical := filepath.Join(layout.Root, "linked")
			if err := os.RemoveAll(canonical); err != nil {
				t.Fatal(err)
			}
			outside := t.TempDir()
			if err := os.WriteFile(filepath.Join(outside, ".git"), nil, 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, canonical); err != nil {
				t.Fatal(err)
			}
			return layout, "linked"
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			setStateDir(t)
			layout, target := setup(t)
			fake := installAgentFakeRex(t, RexState{})
			if _, err := LaunchRexAgent(layout, target, testAgentArgv()); err == nil {
				t.Fatal("expected rejection")
			}
			if calls := fake.calls(t); len(calls) != 0 {
				t.Fatalf("Rex was called before validation: %q", calls)
			}
		})
	}
}

func TestLaunchRexAgentRequiresAbsoluteExecutable(t *testing.T) {
	setStateDir(t)
	layout, _ := agentWorktree(t, "feature-f", WorktreeMeta{})
	fake := installAgentFakeRex(t, RexState{})
	if _, err := LaunchRexAgent(layout, "feature-f", []string{"claude", testAgentBrief}); err == nil {
		t.Fatal("expected relative executable rejection")
	}
	if calls := fake.calls(t); len(calls) != 0 {
		t.Fatalf("Rex was called: %q", calls)
	}
}
