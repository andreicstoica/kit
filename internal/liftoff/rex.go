package liftoff

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const rexTimeout = 5 * time.Second

// RexState is a point-in-time view of sessions, windows (tabs), and blocks (panes).
type RexState struct {
	Sessions []RexSession `json:"sessions"`
}

func (s *RexState) UnmarshalJSON(data []byte) error {
	var raw struct {
		Sessions json.RawMessage `json:"sessions"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	sessions, err := rexArray(raw.Sessions)
	if err != nil {
		return err
	}
	return json.Unmarshal(sessions, &s.Sessions)
}

type RexSession struct {
	SessionID      string      `json:"session_id"`
	Label          string      `json:"label"`
	Windows        []RexWindow `json:"windows"`
	DetachedBlocks []RexBlock  `json:"detached_blocks"`
}

func (s *RexSession) UnmarshalJSON(data []byte) error {
	var raw struct {
		SessionID      string          `json:"session_id"`
		Label          string          `json:"label"`
		Windows        json.RawMessage `json:"windows"`
		DetachedBlocks json.RawMessage `json:"detached_blocks"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	windows, err := rexArray(raw.Windows)
	if err != nil {
		return err
	}
	s.SessionID, s.Label = raw.SessionID, raw.Label
	if err := json.Unmarshal(windows, &s.Windows); err != nil {
		return err
	}
	detached, err := rexArray(raw.DetachedBlocks)
	if err != nil {
		return err
	}
	return json.Unmarshal(detached, &s.DetachedBlocks)
}

type RexWindow struct {
	WindowID string     `json:"window_id"`
	Label    string     `json:"label"`
	Active   bool       `json:"active"`
	Blocks   []RexBlock `json:"blocks"`
}

func (w *RexWindow) UnmarshalJSON(data []byte) error {
	var raw struct {
		WindowID string          `json:"window_id"`
		Label    string          `json:"label"`
		Active   bool            `json:"active"`
		Blocks   json.RawMessage `json:"blocks"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	blocks, err := rexArray(raw.Blocks)
	if err != nil {
		return err
	}
	w.WindowID, w.Label, w.Active = raw.WindowID, raw.Label, raw.Active
	return json.Unmarshal(blocks, &w.Blocks)
}

type RexBlock struct {
	BlockID       string           `json:"block_id"`
	Label         string           `json:"label"`
	Creator       string           `json:"creator_name"`
	Flavor        string           `json:"flavor"`
	Placement     string           `json:"placement"`
	Status        string           `json:"status"`
	ProgramStatus RexProgramStatus `json:"program_status"`
}

func rexArray(raw json.RawMessage) ([]byte, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" || trimmed == "{}" {
		return []byte("[]"), nil
	}
	if strings.HasPrefix(trimmed, "{") {
		return nil, fmt.Errorf("expected Rex array, got object")
	}
	return raw, nil
}

func rexCLIPath() string {
	if path, err := exec.LookPath("rex"); err == nil {
		return path
	}
	for _, path := range []string{"/Applications/Rex.app/Contents/Helpers/rex", "/Applications/Rex Beta.app/Contents/Helpers/rex"} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return path
		}
	}
	return ""
}

func RexAvailable() bool { return rexCLIPath() != "" }

// runRex runs one bounded Rex CLI request. It is intentionally local and has
// no fallback to starting Rex or launching the app when the CLI is unavailable.
func runRex(args ...string) (string, error) {
	path := rexCLIPath()
	if path == "" {
		return "", fmt.Errorf("Rex is unavailable: `rex` was not found on PATH and no supported Rex app bundle was found")
	}
	ctx, cancel := context.WithTimeout(context.Background(), rexTimeout)
	defer cancel()
	cmdArgs := withRexAutostartDisabled(args)
	cmd := exec.CommandContext(ctx, path, cmdArgs...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("Rex CLI timed out after %s", rexTimeout)
	}
	if err != nil {
		message := strings.TrimSpace(string(out))
		if message != "" {
			return "", fmt.Errorf("Rex CLI: %w: %s", err, message)
		}
		return "", fmt.Errorf("Rex CLI: %w", err)
	}
	return string(out), nil
}

func withRexAutostartDisabled(args []string) []string {
	if len(args) == 0 {
		return []string{"--autostart=false"}
	}
	result := append([]string(nil), args...)
	insertAt := len(result)
	for i, arg := range result {
		if arg == "--" {
			insertAt = i
			break
		}
	}
	result = append(result, "")
	copy(result[insertAt+1:], result[insertAt:])
	result[insertAt] = "--autostart=false"
	return result
}

func parseRexState(input string) (RexState, error) {
	var state RexState
	if err := json.Unmarshal([]byte(input), &state); err != nil {
		return RexState{}, fmt.Errorf("parse Rex state: %w", err)
	}
	return state, nil
}

// ReadRexState reads a structural snapshot of every session plus per-block
// process and program-status records, using one Lua request. It attaches the
// script's own short-lived connection to each session it reads, which Rex
// requires, but never creates, closes, or focuses sessions.
func ReadRexState() (RexState, error) { return readRexState("", true) }

// readRexStructure reads session, window, and block identity and labels only.
// It skips the per-block process and program-status requests. A non-empty
// sessionID limits the read to that session.
func readRexStructure(sessionID string) (RexState, error) { return readRexState(sessionID, false) }

func readRexState(sessionID string, status bool) (RexState, error) {
	if !RexAvailable() {
		return RexState{}, fmt.Errorf("Rex is unavailable: `rex` was not found on PATH and no supported Rex app bundle was found")
	}
	args := map[string]any{"status": status}
	if sessionID != "" {
		args["session_id"] = sessionID
	}
	payload, err := json.Marshal(args)
	if err != nil {
		return RexState{}, err
	}
	out, err := runRex("do", "-e", rexSnapshotLua, "--args", string(payload))
	if err != nil {
		return RexState{}, err
	}
	return parseRexState(out)
}

// OpenRex creates or reuses a session and adds only missing Kit layout tabs;
// user-created tabs are never removed. It records the stable session ID.
func OpenRex(name, path, layoutName string) (RexSession, error) {
	var session RexSession
	err := withWorkspaceLock("rex:"+name, func() error {
		var err error
		session, err = openRex(name, path, layoutName)
		return err
	})
	return session, err
}

func openRex(name, path, layoutName string) (RexSession, error) {
	if !RexAvailable() {
		return RexSession{}, fmt.Errorf("Rex is unavailable: `rex` was not found on PATH and no supported Rex app bundle was found")
	}
	if layoutName == "" {
		layoutName = DefaultWorkspaceLayoutName()
	}
	layout, err := ResolveWorkspaceLayout(layoutName)
	if err != nil {
		return RexSession{}, err
	}
	cfg, err := LoadConfig()
	if err != nil {
		return RexSession{}, fmt.Errorf("load Kit config for Rex mapping: %w", err)
	}
	meta, exists := cfg.Worktrees[name]
	savedID := meta.RexID
	shellWindow := meta.RexShellWindow
	if name == "master" {
		savedID = cfg.Settings.RexMasterSession
		shellWindow = cfg.Settings.RexMasterShellWindow
	}
	state, err := readRexStructure("")
	if err != nil {
		return RexSession{}, err
	}
	session := findRexSession(state, savedID, "")
	changed := false
	var windows []RexWindow
	if session == nil {
		if collision := findRexSession(state, "", name); collision != nil {
			return RexSession{}, fmt.Errorf("Rex session label %q is already in use by unrelated session %s; rename that session or explicitly map its ID", name, collision.SessionID)
		}
		if name != "master" && !exists {
			return RexSession{}, fmt.Errorf("cannot create Rex session for %q: no Kit worktree mapping exists", name)
		}
		var createdTabs []string
		var args []string
		if layoutName == "simple" {
			payload, err := json.Marshal(map[string]string{"name": name, "cwd": path, "command": workspaceTabCommand(name, "claude")})
			if err != nil {
				return RexSession{}, err
			}
			args = []string{"do", "-e", rexSimpleSessionLua, "--args", string(payload)}
			createdTabs = []string{"shell"}
		} else {
			var tabs []map[string]string
			// Every Kit session has a shell tab, even when a custom layout omits it.
			for _, tab := range uniqueStrings(append([]string{"shell"}, layout.Tabs...)) {
				tabs = append(tabs, map[string]string{"label": tab, "command": workspaceTabCommand(name, tab)})
				createdTabs = append(createdTabs, tab)
			}
			payload, err := json.Marshal(map[string]any{"name": name, "cwd": path, "tabs": tabs})
			if err != nil {
				return RexSession{}, err
			}
			args = []string{"do", "-e", rexCreateSessionLua, "--args", string(payload)}
		}
		out, e := runRex(args...)
		if e != nil {
			return RexSession{}, fmt.Errorf("create Rex session %q: %w", name, e)
		}
		var created struct {
			SessionID string `json:"session_id"`
		}
		if e := json.Unmarshal([]byte(out), &created); e != nil || created.SessionID == "" {
			return RexSession{}, fmt.Errorf("parse Rex create response: missing session_id")
		}
		savedID = created.SessionID
		if name == "master" {
			if err := WithConfigLock(func(c *Config) error { c.Settings.RexMasterSession = savedID; return nil }); err != nil {
				_, _ = runRex("kill", savedID)
				return RexSession{}, fmt.Errorf("save master Rex session mapping: %w", err)
			}
		} else {
			if err := WithConfigLock(func(c *Config) error {
				m, ok := c.Worktrees[name]
				if !ok {
					return fmt.Errorf("Kit worktree mapping %q disappeared", name)
				}
				m.RexID, m.RexLayout = savedID, layoutName
				c.Worktrees[name] = m
				return nil
			}); err != nil {
				_, _ = runRex("kill", savedID) // an unmapped session would collide on its label forever
				return RexSession{}, fmt.Errorf("save new Rex session mapping: %w", err)
			}
		}
		shellWindow = "" // window IDs of a replaced session are meaningless
		session = &RexSession{SessionID: savedID, Label: name}
		for _, tab := range createdTabs {
			windows = append(windows, RexWindow{Label: tab})
		}
		changed = true
	} else {
		windows = session.Windows
	}
	seen := map[string]bool{}
	for _, w := range windows {
		seen[w.Label] = true
		if layoutName == "simple" {
			seen["shell"] = true // its one tab is unlabeled so Rex can title it
		}
		if shellWindow != "" && w.WindowID == shellWindow {
			seen["shell"] = true
		}
	}
	var missing []map[string]string
	for _, tab := range uniqueStrings(layout.Tabs) {
		if !seen[tab] {
			missing = append(missing, map[string]string{"label": tab, "command": workspaceTabCommand(name, tab)})
		}
	}
	if len(missing) > 0 {
		payload, err := json.Marshal(map[string]any{"session_id": session.SessionID, "cwd": path, "tabs": missing})
		if err != nil {
			return RexSession{}, err
		}
		changed = true
		if _, err := runRex("do", "-e", rexEnsureTabsLua, "--args", string(payload)); err != nil {
			return RexSession{}, err
		}
	}
	if changed {
		state, err = readRexStructure(session.SessionID)
		if err != nil {
			return RexSession{}, err
		}
		session = findRexSession(state, session.SessionID, "")
		if session == nil {
			return RexSession{}, fmt.Errorf("Rex created session %q but it was not present in the state snapshot", name)
		}
	}
	if shellWindow == "" {
		for _, w := range session.Windows {
			if w.Label == "shell" || layoutName == "simple" {
				shellWindow = w.WindowID
				break
			}
		}
	}
	if name == "master" {
		if err := WithConfigLock(func(c *Config) error {
			c.Settings.RexMasterSession = session.SessionID
			c.Settings.RexMasterShellWindow = shellWindow
			return nil
		}); err != nil {
			return RexSession{}, fmt.Errorf("save master Rex mapping: %w", err)
		}
	} else if exists {
		if err := WithConfigLock(func(c *Config) error {
			m, ok := c.Worktrees[name]
			if !ok {
				return fmt.Errorf("Kit worktree mapping %q disappeared", name)
			}
			m.RexID = session.SessionID
			m.RexLayout = layoutName
			m.RexShellWindow = shellWindow
			c.Worktrees[name] = m
			return nil
		}); err != nil {
			return RexSession{}, fmt.Errorf("save Rex mapping: %w", err)
		}
	}
	return *session, nil
}

func findRexSession(state RexState, id, name string) *RexSession {
	for i := range state.Sessions {
		if id != "" && state.Sessions[i].SessionID == id {
			return &state.Sessions[i]
		}
	}
	for i := range state.Sessions {
		if name != "" && state.Sessions[i].Label == name {
			return &state.Sessions[i]
		}
	}
	return nil
}

// CloseRex explicitly closes the mapped session. Missing sessions are harmless.
func CloseRex(name, path string) error {
	return withWorkspaceLock("rex:"+name, func() error {
		if err := closeImportedRexWorkspaces(path); err != nil {
			return err
		}
		return closeRex(name, path)
	})
}

func closeRex(name, path string) error {
	if !RexAvailable() {
		return fmt.Errorf("Rex is unavailable: `rex` was not found on PATH and no supported Rex app bundle was found")
	}
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("load Kit config for Rex mapping: %w", err)
	}
	if name == "master" {
		id := cfg.Settings.RexMasterSession
		if id == "" {
			return nil
		}
		state, err := readRexStructure(id)
		if err != nil {
			return err
		}
		if findRexSession(state, id, "") == nil {
			return nil
		}
		if _, err := runRex("kill", id); err != nil {
			return fmt.Errorf("close Rex master session: %w", err)
		}
		return WithConfigLock(func(c *Config) error {
			c.Settings.RexMasterSession = ""
			c.Settings.RexMasterShellWindow = ""
			return nil
		})
	}
	meta, exists := cfg.Worktrees[name]
	if !exists || meta.RexID == "" {
		return nil
	}
	id := meta.RexID
	state, err := readRexStructure(id)
	if err != nil {
		return err
	}
	s := findRexSession(state, id, "")
	if s != nil {
		if _, err := runRex("kill", s.SessionID); err != nil {
			return fmt.Errorf("close Rex session %q: %w", name, err)
		}
	}
	return WithConfigLock(func(c *Config) error {
		m, ok := c.Worktrees[name]
		if !ok {
			return nil
		}
		m.RexID = ""
		m.RexLayout = ""
		m.RexShellWindow = ""
		c.Worktrees[name] = m
		return nil
	})
}

// FocusRexClient selects the active window of a session. Rex's server applies
// this focus to attached clients; foregrounding the app itself is platform UI work.
func FocusRexClient(sessionID string) error {
	state, err := readRexStructure(sessionID)
	if err != nil {
		return err
	}
	for _, s := range state.Sessions {
		if s.SessionID == sessionID {
			for _, w := range s.Windows {
				if w.Active {
					return selectRexAppSession(sessionID, w.WindowID)
				}
			}
			return fmt.Errorf("Rex session %q has no active window", sessionID)
		}
	}
	return fmt.Errorf("Rex session %q not found", sessionID)
}

// Server-side pane focus does not switch the session shown by the native app.
// Route the discovered session.select action to an actual GUI client instead.
func selectRexAppSession(sessionID, windowID string) error {
	out, err := runRex("client", "list", "--json")
	if err != nil {
		return err
	}
	var list struct {
		Clients []struct {
			ID   string `json:"client_id"`
			Info struct {
				Kind string `json:"kind"`
			} `json:"info"`
		} `json:"clients"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		return fmt.Errorf("parse Rex clients: %w", err)
	}
	var clientID string
	for _, client := range list.Clients {
		if client.Info.Kind != "app" {
			continue
		}
		if clientID != "" {
			return fmt.Errorf("multiple Rex app clients are connected; choose the workspace in the app")
		}
		clientID = client.ID
	}
	if clientID == "" {
		return fmt.Errorf("Rex workspace is ready; open the Rex app to view it")
	}
	_, err = runRex("-C", clientID, "do", "session.select", "session_id="+sessionID, "window_id="+windowID)
	if err != nil {
		return fmt.Errorf("select Rex workspace in app (enable Remote Control in Rex settings): %w", err)
	}
	return nil
}
