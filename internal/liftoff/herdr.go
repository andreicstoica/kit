package liftoff

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// HerdrState is the JSON snapshot returned by `herdr api snapshot`. Kit uses
// the public snapshot rather than Herdr's on-disk files so Herdr remains an
// implementation detail and can change its persistence format independently.
type HerdrState struct {
	Workspaces []HerdrWorkspace `json:"workspaces"`
	Tabs       []HerdrTab       `json:"tabs"`
	Panes      []HerdrPane      `json:"panes"`
}

type HerdrWorkspace struct {
	WorkspaceID string            `json:"workspace_id"`
	Label       string            `json:"label"`
	PaneCount   int               `json:"pane_count"`
	TabCount    int               `json:"tab_count"`
	AgentStatus string            `json:"agent_status"`
	Worktree    *HerdrWorktreeRef `json:"worktree"`
}

type HerdrWorktreeRef struct {
	CheckoutPath string `json:"checkout_path"`
}

type HerdrTab struct {
	TabID       string `json:"tab_id"`
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
}

type HerdrPane struct {
	PaneID      string  `json:"pane_id"`
	TabID       string  `json:"tab_id"`
	WorkspaceID string  `json:"workspace_id"`
	CWD         *string `json:"cwd"`
	Agent       *string `json:"agent"`
	AgentStatus string  `json:"agent_status"`
}

const defaultHerdrSession = "default"

// BuiltinHerdrLayouts are the Kit-owned layouts. The first two preserve the
// existing Ghostty workflow: simple is the emoji/worktree shell tab + logs,
// detailed is the old five-tab frontend/backend/celery/logs layout. AI is a
// forward-looking layout that keeps each agent in its own durable tab.
func BuiltinHerdrLayouts() map[string]HerdrLayout {
	return map[string]HerdrLayout{
		"default":  {Tabs: []string{"shell", "logs"}},
		"simple":   {Tabs: []string{"shell", "logs"}},
		"detailed": {Tabs: []string{"shell", "frontend", "backend", "celery", "logs"}},
		"ai":       {Tabs: []string{"shell", "claude", "codex", "gemini", "logs"}},
	}
}

// HerdrShellTabLabel is the stable root-tab label shared by Herdr and the
// legacy Ghostty workspace. The worktree name identifies the workspace; the
// emoji makes the root tab recognizable when several workspaces are attached.
func HerdrShellTabLabel(name string) string {
	if emoji := EmojiFor(name); emoji != "" {
		return emoji + " " + name
	}
	return name
}

// HerdrSessionName returns the named persistent Herdr session Kit uses. The
// environment override is useful for remote hosts; config makes the choice
// durable for normal local use.
func HerdrSessionName() string {
	if v := os.Getenv("HERDR_SESSION"); v != "" {
		return v
	}
	if v := os.Getenv("KIT_HERDR_SESSION"); v != "" {
		return v
	}
	if c, err := LoadConfig(); err == nil && c.Settings.HerdrSession != "" {
		return c.Settings.HerdrSession
	}
	return defaultHerdrSession
}

// DefaultHerdrLayoutName returns the configured default layout, falling back
// to the simple layout that mirrors the old Ghostty workspace.
func DefaultHerdrLayoutName() string {
	if v := os.Getenv("KIT_HERDR_LAYOUT"); v != "" {
		return v
	}
	if c, err := LoadConfig(); err == nil && c.Settings.HerdrLayout != "" {
		return c.Settings.HerdrLayout
	}
	return "default"
}

// ResolveHerdrLayout merges user-defined layouts over Kit's built-ins. This
// lets config.toml contain, for example:
//
//	[layouts.default]
//	tabs = ["shell", "claude", "codex", "logs"]
func ResolveHerdrLayout(name string) (HerdrLayout, error) {
	if name == "" {
		name = DefaultHerdrLayoutName()
	}
	layouts := BuiltinHerdrLayouts()
	if c, err := LoadConfig(); err == nil {
		for key, value := range c.Layouts {
			layouts[key] = value
		}
	}
	layout, ok := layouts[name]
	if !ok || len(layout.Tabs) == 0 {
		return HerdrLayout{}, fmt.Errorf("unknown Herdr layout %q", name)
	}
	return layout, nil
}

// herdrSnapshotEnvelope matches the JSON-RPC style response Herdr's CLI emits
// (`{"id":…,"result":{"snapshot":{…}}}`). Older builds returned the snapshot
// object at the top level, so both shapes are accepted.
type herdrSnapshotEnvelope struct {
	Result *struct {
		Snapshot *HerdrState `json:"snapshot"`
	} `json:"result"`
}

// ReadHerdrState reads the public runtime snapshot. It does not start Herdr;
// callers that need a usable server should use OpenHerdr or EnsureHerdr.
func ReadHerdrState() (HerdrState, error) {
	out, err := runHerdr("api", "snapshot")
	if err != nil {
		return HerdrState{}, err
	}
	return parseHerdrSnapshot(out)
}

// parseHerdrSnapshot decodes either the enveloped or the legacy flat snapshot.
//
// An envelope whose payload we cannot reach is a hard error rather than an
// empty state: because encoding/json ignores unknown fields, reading the flat
// shape out of an enveloped response used to yield zero workspaces with no
// error, which callers could only read as "this workspace doesn't exist" —
// so a Herdr format change surfaced as a bogus "created it but it isn't in
// the snapshot" failure instead of a parse problem.
func parseHerdrSnapshot(out string) (HerdrState, error) {
	var envelope herdrSnapshotEnvelope
	if err := json.Unmarshal([]byte(out), &envelope); err == nil && envelope.Result != nil {
		if envelope.Result.Snapshot == nil {
			return HerdrState{}, fmt.Errorf("parse Herdr snapshot: response had no result.snapshot payload")
		}
		return *envelope.Result.Snapshot, nil
	}

	var state HerdrState
	if err := json.Unmarshal([]byte(out), &state); err != nil {
		return HerdrState{}, fmt.Errorf("parse Herdr snapshot: %w", err)
	}
	return state, nil
}

// HerdrAvailable reports whether the adapter can find the Herdr executable.
func HerdrAvailable() bool {
	_, err := exec.LookPath("herdr")
	return err == nil
}

// FindHerdrWorkspace matches a Kit worktree to its durable Herdr workspace.
// The saved ID is the strongest key. The checkout path and exact label make
// the mapping recoverable if Herdr restores IDs or Kit config from a backup.
func FindHerdrWorkspace(state HerdrState, savedID, name, path string) *HerdrWorkspace {
	if savedID != "" {
		for i := range state.Workspaces {
			if state.Workspaces[i].WorkspaceID == savedID {
				return &state.Workspaces[i]
			}
		}
	}
	path = cleanPath(path)
	for i := range state.Workspaces {
		w := &state.Workspaces[i]
		if w.Worktree != nil && cleanPath(w.Worktree.CheckoutPath) == path {
			return w
		}
	}
	for i := range state.Workspaces {
		w := &state.Workspaces[i]
		if !herdrLabelMatches(w.Label, name) {
			continue
		}
		if workspaceHasPath(state, w.WorkspaceID, path) {
			return w
		}
	}
	for i := range state.Workspaces {
		if herdrLabelMatches(state.Workspaces[i].Label, name) {
			return &state.Workspaces[i]
		}
	}
	return nil
}

// HerdrWorkspaceExists reports whether this worktree already has a durable
// Herdr workspace. It is intentionally separate from OpenHerdr so callers
// can offer first-open choices without prompting when reconnecting.
func HerdrWorkspaceExists(name, path string) (bool, error) {
	if !HerdrAvailable() {
		return false, fmt.Errorf("Herdr is not installed; install it with `brew install herdr`")
	}
	if err := EnsureHerdrServer(); err != nil {
		return false, err
	}
	state, err := ReadHerdrState()
	if err != nil {
		return false, err
	}
	var savedID string
	if cfg, loadErr := LoadConfig(); loadErr == nil {
		if meta, ok := cfg.Worktrees[name]; ok {
			savedID = meta.HerdrID
		}
	}
	return FindHerdrWorkspace(state, savedID, name, path) != nil, nil
}

// WorkspaceAgentSummary returns a compact status for lineup/remote views.
func (s HerdrState) WorkspaceAgentSummary(workspaceID string) string {
	var agents []string
	for _, pane := range s.Panes {
		if pane.WorkspaceID != workspaceID || pane.Agent == nil || *pane.Agent == "" {
			continue
		}
		status := pane.AgentStatus
		if status == "" {
			status = "unknown"
		}
		agents = append(agents, *pane.Agent+" "+status)
	}
	if len(agents) == 0 {
		return "idle"
	}
	return strings.Join(agents, ", ")
}

// TruncateHerdrStatus keeps lineup rows readable when several agents share a
// workspace. It is exported so the TUI does not need to know Herdr's shape.
func TruncateHerdrStatus(status string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(status)
	if len(runes) <= max {
		return status
	}
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}

// OpenHerdr ensures the worktree's Herdr workspace and Kit-owned layout,
// focuses it, and records the mapping. It does not attach the current
// terminal; use AttachHerdr for the user-facing `kit open` command.
func OpenHerdr(name, path, layoutName string) (HerdrWorkspace, error) {
	if !HerdrAvailable() {
		return HerdrWorkspace{}, fmt.Errorf("Herdr is not installed; install it with `brew install herdr`")
	}
	if layoutName == "" {
		layoutName = DefaultHerdrLayoutName()
	}
	if _, err := ResolveHerdrLayout(layoutName); err != nil {
		return HerdrWorkspace{}, err
	}
	if err := EnsureHerdrServer(); err != nil {
		return HerdrWorkspace{}, err
	}

	state, err := ReadHerdrState()
	if err != nil {
		return HerdrWorkspace{}, err
	}
	cfg, _ := LoadConfig()
	var savedID string
	if cfg != nil {
		if meta, ok := cfg.Worktrees[name]; ok {
			savedID = meta.HerdrID
		}
	}
	label := HerdrShellTabLabel(name)
	workspace := FindHerdrWorkspace(state, savedID, name, path)
	if workspace == nil {
		if _, err := runHerdr("workspace", "create", "--cwd", path, "--label", label, "--no-focus"); err != nil {
			return HerdrWorkspace{}, fmt.Errorf("create Herdr workspace %q: %w", name, err)
		}
		state, err = ReadHerdrState()
		if err != nil {
			return HerdrWorkspace{}, err
		}
		workspace = FindHerdrWorkspace(state, "", name, path)
		if workspace == nil {
			return HerdrWorkspace{}, fmt.Errorf("Herdr created workspace %q but it was not present in the runtime snapshot", name)
		}
	} else if workspace.Label != label && herdrLabelMatches(workspace.Label, name) {
		// Adopt the emoji prefix on spaces created before it existed. Guarded by
		// herdrLabelMatches so a space the user renamed by hand is left alone.
		if _, err := runHerdr("workspace", "rename", workspace.WorkspaceID, label); err == nil {
			workspace.Label = label
		}
	}

	if err := materializeHerdrLayout(name, path, workspace.WorkspaceID, layoutName, state); err != nil {
		return HerdrWorkspace{}, err
	}
	if _, err := runHerdr("workspace", "focus", workspace.WorkspaceID); err != nil {
		return HerdrWorkspace{}, fmt.Errorf("focus Herdr workspace %q: %w", name, err)
	}

	now := time.Now().UTC()
	if name != "master" {
		if err := WithConfigLock(func(c *Config) error {
			meta := c.Worktrees[name]
			meta.HerdrSpace = name
			meta.HerdrID = workspace.WorkspaceID
			meta.HerdrLayout = layoutName
			meta.LastOpened = now
			meta.LastUsed = now
			c.Worktrees[name] = meta
			return nil
		}); err != nil {
			return HerdrWorkspace{}, fmt.Errorf("save Herdr mapping: %w", err)
		}
	}
	return *workspace, nil
}

// CloseHerdr explicitly deletes a Kit-managed Herdr workspace while leaving
// the Git worktree and its Kit service metadata intact. `kit close` is the
// destructive boundary for terminal state; ordinary open/attach operations
// never remove spaces or tabs.
func CloseHerdr(name, path string) error {
	if !HerdrAvailable() {
		return fmt.Errorf("Herdr is not installed; install it with `brew install herdr`")
	}
	if err := EnsureHerdrServer(); err != nil {
		return err
	}
	cfg, _ := LoadConfig()
	var savedID string
	if cfg != nil {
		if meta, ok := cfg.Worktrees[name]; ok {
			savedID = meta.HerdrID
		}
	}
	state, err := ReadHerdrState()
	if err != nil {
		return err
	}
	workspace := FindHerdrWorkspace(state, savedID, name, path)
	if workspace != nil {
		if _, err := runHerdr("workspace", "close", workspace.WorkspaceID); err != nil {
			return fmt.Errorf("close Herdr workspace %q: %w", name, err)
		}
	}
	if name != "master" {
		if err := WithConfigLock(func(c *Config) error {
			meta := c.Worktrees[name]
			meta.HerdrSpace = ""
			meta.HerdrID = ""
			meta.HerdrLayout = ""
			meta.LastOpened = time.Time{}
			meta.PreferredAgents = nil
			c.Worktrees[name] = meta
			return nil
		}); err != nil {
			return fmt.Errorf("clear Herdr mapping: %w", err)
		}
	}
	return nil
}

// AttachHerdr attaches the current terminal to Kit's shared Herdr session.
// Herdr owns the interactive lifetime; the command returns when the client
// detaches, while all panes and agents continue running in the server.
func AttachHerdr() error {
	cmd := newHerdrCommand("--session", HerdrSessionName())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("attach to Herdr: %w", err)
	}
	return nil
}

// LaunchHerdrInGhostty opens a new Ghostty client whose first process is the
// shared Herdr session. Ghostty is only a client here; workspace/tab state
// remains in Herdr.
func LaunchHerdrInGhostty() error {
	args := []string{"-e", "herdr", "--session", HerdrSessionName()}
	if appBundleExists("Ghostty.app") {
		cmd := exec.Command("open", "-na", "Ghostty.app", "--args")
		cmd.Args = append(cmd.Args, args...)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("open Herdr in Ghostty: %w", err)
		}
		return nil
	}
	if _, err := exec.LookPath("open"); err == nil {
		return fmt.Errorf("Ghostty.app not found; install Ghostty or run `kit open` in the current terminal")
	}
	cmd := exec.Command("ghostty", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}

// FocusHerdrClient focuses a workspace in an already-running Herdr client.
func FocusHerdrClient(workspaceID string) error {
	if _, err := runHerdr("workspace", "focus", workspaceID); err != nil {
		return err
	}
	return nil
}

// EnsureHerdrServer starts a headless Herdr server when no server is
// reachable. This keeps `kit open` self-contained while leaving Herdr's
// process/session persistence in charge of the actual terminal runtime.
func EnsureHerdrServer() error {
	if _, err := ReadHerdrState(); err == nil {
		return nil
	}
	cmd := newHerdrCommand("server")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start Herdr server: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	deadline := time.Now().Add(3 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if _, err := ReadHerdrState(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("server did not become reachable")
	}
	return fmt.Errorf("Herdr server is unavailable: %w", lastErr)
}

func materializeHerdrLayout(name, path, workspaceID, layoutName string, state HerdrState) error {
	layout, err := ResolveHerdrLayout(layoutName)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	var workspaceTabs []HerdrTab
	shellLabel := HerdrShellTabLabel(name)
	for i := range state.Tabs {
		tab := &state.Tabs[i]
		if tab.WorkspaceID == workspaceID {
			workspaceTabs = append(workspaceTabs, *tab)
			switch tab.Label {
			case "shell":
				// Migrate spaces created before v0.3.1 in place.
				if _, err := runHerdr("tab", "rename", tab.TabID, shellLabel); err != nil {
					return fmt.Errorf("name Herdr shell tab: %w", err)
				}
				tab.Label = shellLabel
				seen["shell"] = true
				seen[shellLabel] = true
			case shellLabel:
				seen["shell"] = true
				seen[shellLabel] = true
			default:
				seen[tab.Label] = true
			}
		}
	}
	// workspace create normally supplies the first tab. Give it the legacy
	// Ghostty-compatible label so subsequent opens can recognize the layout
	// without creating duplicates.
	if !seen["shell"] && len(workspaceTabs) == 1 {
		if _, err := runHerdr("tab", "rename", workspaceTabs[0].TabID, shellLabel); err != nil {
			return fmt.Errorf("name Herdr shell tab: %w", err)
		}
		seen["shell"] = true
		seen[shellLabel] = true
	}
	for _, tabName := range uniqueStrings(layout.Tabs) {
		if seen[tabName] {
			continue
		}
		if _, err := runHerdr("tab", "create", "--workspace", workspaceID, "--cwd", path, "--label", tabName, "--no-focus"); err != nil {
			return fmt.Errorf("create Herdr %s tab: %w", tabName, err)
		}
		seen[tabName] = true
		state, err = ReadHerdrState()
		if err != nil {
			return err
		}
		tab := findHerdrTab(state, workspaceID, tabName)
		if tab == nil {
			return fmt.Errorf("Herdr created %s tab but it was not present in the runtime snapshot", tabName)
		}
		if commands := herdrTabPaneCommands(name, tabName); len(commands) > 0 {
			pane := findHerdrPane(state, tab.TabID)
			if pane == nil {
				return fmt.Errorf("Herdr tab %s has no root pane", tabName)
			}
			if _, err := runHerdr("pane", "run", pane.PaneID, commands[0]); err != nil {
				return fmt.Errorf("start Herdr %s tab: %w", tabName, err)
			}
			if len(commands) > 1 {
				if _, err := runHerdr("pane", "split", pane.PaneID, "--direction", "right", "--cwd", path, "--no-focus"); err != nil {
					return fmt.Errorf("split Herdr %s tab: %w", tabName, err)
				}
				state, err = ReadHerdrState()
				if err != nil {
					return err
				}
				second := findHerdrPaneExcept(state, tab.TabID, pane.PaneID)
				if second == nil {
					return fmt.Errorf("Herdr split %s tab has no second pane", tabName)
				}
				if _, err := runHerdr("pane", "run", second.PaneID, commands[1]); err != nil {
					return fmt.Errorf("start Herdr %s split: %w", tabName, err)
				}
			}
		}
	}
	return nil
}

func herdrTabPaneCommands(name, tabName string) []string {
	switch tabName {
	case "frontend":
		return []string{herdrTailCommand(name, SvcApp), herdrTailCommand(name, SvcAdmin)}
	case "backend":
		return []string{herdrTailCommand(name, SvcAPI), herdrTailCommand(name, SvcAdminBE)}
	default:
		if command := herdrTabCommand(name, "", tabName); command != "" {
			return []string{command}
		}
		return nil
	}
}

func herdrTabCommand(name, path, tabName string) string {
	shell := func(agent string) string {
		return fmt.Sprintf("if command -v %s >/dev/null 2>&1; then exec %s; else printf 'Kit: %s is not installed\\n'; exec \"${SHELL:-/bin/sh}\"; fi", agent, agent, agent)
	}
	switch tabName {
	case "logs":
		bin, err := ResolvedExecutable()
		if err != nil || bin == "" {
			bin = "kit"
		}
		return shellQuote(bin) + " log " + shellQuote(name) + " --wait"
	case "frontend":
		return herdrCombinedTailCommand(name, SvcApp, SvcAdmin)
	case "backend":
		return herdrCombinedTailCommand(name, SvcAPI, SvcAdminBE)
	case "celery":
		return herdrTailCommand(name, SvcCelery)
	case "claude", "codex", "gemini":
		return shell(tabName)
	default:
		return ""
	}
}

func herdrTailCommand(name string, service Service) string {
	file, err := LogFile(name, string(service))
	if err != nil {
		return "exec ${SHELL:-/bin/sh}"
	}
	return "tail -F " + shellQuote(file)
}

func herdrCombinedTailCommand(name string, services ...Service) string {
	files := make([]string, 0, len(services))
	for _, service := range services {
		file, err := LogFile(name, string(service))
		if err == nil {
			files = append(files, shellQuote(file))
		}
	}
	if len(files) == 0 {
		return "exec ${SHELL:-/bin/sh}"
	}
	return "tail -F " + strings.Join(files, " ")
}

func findHerdrTab(state HerdrState, workspaceID, label string) *HerdrTab {
	for i := range state.Tabs {
		if state.Tabs[i].WorkspaceID == workspaceID && state.Tabs[i].Label == label {
			return &state.Tabs[i]
		}
	}
	return nil
}

func findHerdrPane(state HerdrState, tabID string) *HerdrPane {
	for i := range state.Panes {
		if state.Panes[i].TabID == tabID {
			return &state.Panes[i]
		}
	}
	return nil
}

func findHerdrPaneExcept(state HerdrState, tabID, paneID string) *HerdrPane {
	for i := range state.Panes {
		if state.Panes[i].TabID == tabID && state.Panes[i].PaneID != paneID {
			return &state.Panes[i]
		}
	}
	return nil
}

func workspaceHasPath(state HerdrState, workspaceID, path string) bool {
	for _, pane := range state.Panes {
		if pane.WorkspaceID != workspaceID || pane.CWD == nil {
			continue
		}
		if cleanPath(*pane.CWD) == path || isPathInside(*pane.CWD, path) {
			return true
		}
	}
	return false
}

func herdrLabelMatches(label, name string) bool {
	return label == name || strings.HasSuffix(label, " "+name)
}

func cleanPath(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

func isPathInside(path, root string) bool {
	path, root = cleanPath(path), cleanPath(root)
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func runHerdr(args ...string) (string, error) {
	cmd := newHerdrCommand(args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(out))
		if message == "" {
			return "", err
		}
		return "", fmt.Errorf("%w: %s", err, message)
	}
	return string(out), nil
}

func newHerdrCommand(args ...string) *exec.Cmd {
	cmd := exec.Command("herdr", args...)
	cmd.Env = append(os.Environ(), "HERDR_SESSION="+HerdrSessionName())
	return cmd
}
