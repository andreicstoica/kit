package liftoff

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// rexAgentWindowLabel names launched agent windows. It must not match a layout
// tab name such as "claude" or the unlabeled shell tab, or a later OpenRex
// would mistake the agent window for a Kit layout tab.
const rexAgentWindowLabel = "agent"

// RexAgentLaunch describes an agent window that Rex created. When FocusErr is
// set, the window is open but the Rex app does not show it; callers must check
// that window before launching again.
type RexAgentLaunch struct {
	Name      string
	Path      string
	SessionID string
	WindowID  string
	FocusErr  error
}

// ResolveAgentWorktree returns the real directory of an existing Kit feature
// worktree that may receive a new agent. It refuses master, unknown names,
// parked or cleanup-pending worktrees, and paths outside Kit's mapping.
func ResolveAgentWorktree(layout Layout, name string) (string, error) {
	// Adopted worktrees keep their directory basename, so names need not be
	// kebab-case. Exact config membership plus the path check below is the
	// boundary; this only rejects names that could address another path.
	if name == "" || name == "." || name == ".." || name == "master" || name == "main" || strings.ContainsAny(name, "/\\\x00") {
		return "", fmt.Errorf("invalid worktree name %q: name a Kit feature worktree", name)
	}
	cfg, err := LoadConfig()
	if err != nil {
		return "", fmt.Errorf("load Kit config: %w", err)
	}
	meta, ok := cfg.Worktrees[name]
	if !ok {
		return "", fmt.Errorf("no Kit worktree named %q; create it with kit design first", name)
	}
	if meta.Parked {
		return "", fmt.Errorf("%s is parked; run kit resume %s first", name, name)
	}
	if meta.CleanupPending || meta.Cleanup != nil {
		return "", fmt.Errorf("%s has pending cleanup; finish or cancel it before launching an agent", name)
	}
	path, err := layout.ResolveWorktreePath(name)
	if err != nil {
		return "", err
	}
	if info, err := os.Lstat(path); err != nil {
		return "", fmt.Errorf("worktree directory: %w", err)
	} else if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("worktree path %s is a symlink; refusing to launch an agent through it", path)
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("worktree directory: %w", err)
	}
	if err := layout.VerifyGitCheckout(name, real, meta.Branch); err != nil {
		return "", err
	}
	return real, nil
}

// LaunchRexAgent opens argv in a new window of the worktree's Kit-managed Rex
// session and selects that window in the Rex app. argv runs without a shell.
// A *RexAgentUnconfirmedError means the window may exist; any other error
// means it was not opened. After the window exists, selection problems are
// reported only through FocusErr.
func LaunchRexAgent(layout Layout, name string, argv []string) (RexAgentLaunch, error) {
	if len(argv) == 0 || !filepath.IsAbs(argv[0]) {
		return RexAgentLaunch{}, fmt.Errorf("agent command must start with an absolute executable path")
	}
	if !RexAvailable() {
		return RexAgentLaunch{}, fmt.Errorf("Rex is unavailable: `rex` was not found on PATH and no supported Rex app bundle was found")
	}
	// Prefer exact adopted names, then accept the same aliases as design.
	if normalized := Normalize(name); normalized != name {
		cfg, err := LoadConfig()
		if err != nil {
			return RexAgentLaunch{}, err
		}
		if _, exact := cfg.Worktrees[name]; !exact && Validate(normalized) == nil {
			name = normalized
		}
	}
	var result RexAgentLaunch
	err := withWorkspaceLock("rex:"+name, func() error {
		path, err := ResolveAgentWorktree(layout, name)
		if err != nil {
			return err
		}
		session, err := agentRexSession(name, path)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(map[string]any{
			"session_id": session.SessionID,
			"cwd":        path,
			"label":      rexAgentWindowLabel,
			"argv":       argv,
		})
		if err != nil {
			return err
		}
		out, runErr := runRex("do", "-s", session.SessionID, "-e", rexLaunchAgentLua, "--args", string(payload))
		var created struct {
			WindowID string `json:"window_id"`
		}
		if runErr == nil {
			_ = json.Unmarshal([]byte(out), &created)
		}
		windowID := created.WindowID
		// A snapshot cannot distinguish a late launch from an unrelated window.
		// Never include Rex's raw output here: it can contain the brief.
		if windowID == "" {
			return &RexAgentUnconfirmedError{SessionID: session.SessionID}
		}
		result = RexAgentLaunch{Name: name, Path: path, SessionID: session.SessionID, WindowID: windowID}
		result.FocusErr = selectRexAppSession(session.SessionID, windowID)
		return nil
	})
	return result, err
}

// agentRexSession reuses a live mapped session without adding layout tabs, so
// a layout with its own agent tab cannot start a second agent. Without a live
// session it creates one with the shell-only default layout.
func agentRexSession(name, path string) (RexSession, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return RexSession{}, fmt.Errorf("load Kit config for Rex mapping: %w", err)
	}
	if id := cfg.Worktrees[name].RexID; id != "" {
		state, err := readRexStructure(id)
		if err != nil {
			return RexSession{}, err
		}
		if session := findRexSession(state, id, ""); session != nil {
			return *session, nil
		}
	}
	return openRex(name, path, "default")
}

// RexAgentUnconfirmedError means the Rex request failed and Kit could not tell
// whether the agent window opened. Retrying blindly can start a duplicate.
type RexAgentUnconfirmedError struct{ SessionID string }

func (e *RexAgentUnconfirmedError) Error() string {
	return fmt.Sprintf("Kit could not confirm whether Rex opened the agent window in session %s; check that session in the Rex app before launching again", e.SessionID)
}
