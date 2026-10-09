package liftoff

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RexImportMap records the stable source-to-destination identity for retries.
// It intentionally contains no terminal contents, commands, or process data.
type RexImportMap struct {
	Workspaces map[string]string `json:"workspaces"`
	Tabs       map[string]string `json:"tabs"`
	Panes      map[string]string `json:"panes"`
	Checkouts  map[string]string `json:"checkouts,omitempty"`
}

func rexImportMapPath() string { return filepath.Join(configDir(), "rex-import.json") }

func LoadRexImportMap() (RexImportMap, error) {
	m := RexImportMap{Workspaces: map[string]string{}, Tabs: map[string]string{}, Panes: map[string]string{}}
	b, err := os.ReadFile(rexImportMapPath())
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, fmt.Errorf("parse Rex import map: %w", err)
	}
	if m.Workspaces == nil {
		m.Workspaces = map[string]string{}
	}
	if m.Tabs == nil {
		m.Tabs = map[string]string{}
	}
	if m.Panes == nil {
		m.Panes = map[string]string{}
	}
	if m.Checkouts == nil {
		m.Checkouts = map[string]string{}
	}
	return m, nil
}

// SaveRexImportMap atomically persists IDs and checkout ownership. The private file contains no
// pane content, prompts, command lines, or terminal output.
func SaveRexImportMap(m RexImportMap) error {
	path := rexImportMapPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Chmod(path, 0o600)
}

// RexImportItem is a safe, declarative reconstruction of one Herdr workspace.
// Panes are recreated as shells; live processes cannot be transferred.
type RexImportItem struct {
	SourceID string
	Label    string
	CWD      string
	Tabs     []RexImportTab
	Warnings []string
	Skip     string
	Blocked  bool // Safety decision; never infer it from the display text in Skip.
}

type RexImportTab struct {
	SourceID string
	Label    string
	Panes    []RexImportPane
}

type RexImportPane struct {
	SourceID   string
	CWD        string
	Agent      string
	KnownAgent bool
}

// PlanRexImport plans reconstruction from a Herdr snapshot without reading pane
// output or starting processes. Existing Rex workspace IDs are protected unless
// they are already the recorded destination for this source (an idempotent retry).
func PlanRexImport(source HerdrState, mapping RexImportMap, existingIDs map[string]bool) []RexImportItem {
	if mapping.Workspaces == nil {
		mapping.Workspaces = map[string]string{}
	}
	tabsByWorkspace := map[string][]HerdrTab{}
	panesByTab := map[string][]HerdrPane{}
	for _, tab := range source.Tabs {
		tabsByWorkspace[tab.WorkspaceID] = append(tabsByWorkspace[tab.WorkspaceID], tab)
	}
	for _, pane := range source.Panes {
		panesByTab[pane.TabID] = append(panesByTab[pane.TabID], pane)
	}

	items := make([]RexImportItem, 0, len(source.Workspaces))
	for _, ws := range source.Workspaces {
		item := RexImportItem{SourceID: ws.WorkspaceID, Label: ws.Label}
		item.CWD = workspaceImportRoot(source, ws)
		if mapped := mapping.Workspaces[ws.WorkspaceID]; mapped != "" {
			if existingIDs[mapped] {
				item.Skip = "mapped to an existing Rex session; retry will continue missing tabs/panes"
			} else {
				item.Skip = "saved Rex destination is missing; apply refuses automatic recreation"
				item.Blocked = true
			}
		} else if existingIDs[ws.WorkspaceID] {
			item.Skip = "destination ID collision; no mapping exists"
			item.Blocked = true
		}
		if item.CWD == "" || !dirExists(item.CWD) {
			item.Warnings = append(item.Warnings, "workspace checkout path is unavailable; choose a valid directory before applying")
		}
		for _, tab := range tabsByWorkspace[ws.WorkspaceID] {
			planned := RexImportTab{SourceID: tab.TabID, Label: tab.Label}
			for _, pane := range panesByTab[tab.TabID] {
				p := RexImportPane{SourceID: pane.PaneID}
				if pane.CWD != nil {
					p.CWD = cleanPath(*pane.CWD)
				}
				if pane.Agent != nil {
					p.Agent = strings.TrimSpace(*pane.Agent)
				}
				p.KnownAgent = knownHerdrAgent(p.Agent)
				if p.CWD == "" || !dirExists(p.CWD) {
					p.CWD = item.CWD
					if p.CWD == "" || !dirExists(p.CWD) {
						item.Warnings = append(item.Warnings, fmt.Sprintf("pane %s has no available working directory", pane.PaneID))
					}
				} else if !isPathInside(p.CWD, item.CWD) && item.CWD != "" {
					item.Warnings = append(item.Warnings, fmt.Sprintf("pane %s cwd is outside its workspace checkout", pane.PaneID))
				}
				if p.Agent != "" && !p.KnownAgent {
					item.Warnings = append(item.Warnings, fmt.Sprintf("pane %s uses unknown agent %q; import as shell", pane.PaneID, p.Agent))
				}
				planned.Panes = append(planned.Panes, p)
			}
			item.Tabs = append(item.Tabs, planned)
		}
		items = append(items, item)
	}
	return items
}

func knownHerdrAgent(name string) bool {
	switch strings.ToLower(name) {
	case "claude", "codex", "gemini", "opencode":
		return true
	default:
		return false
	}
}

func dirExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(filepath.Clean(path))
	return err == nil && info.IsDir()
}

func workspaceImportRoot(source HerdrState, ws HerdrWorkspace) string {
	if ws.Worktree != nil && dirExists(ws.Worktree.CheckoutPath) {
		return cleanPath(ws.Worktree.CheckoutPath)
	}
	for _, pane := range source.Panes {
		if pane.WorkspaceID == ws.WorkspaceID && pane.CWD != nil && dirExists(*pane.CWD) {
			return cleanPath(*pane.CWD)
		}
	}
	return ""
}

// AnnotateRexImportCollisions mirrors ApplyRexImport's refusal to adopt an
// unrelated same-label Rex session. The dry-run can therefore predict refusal.
func AnnotateRexImportCollisions(items []RexImportItem, state RexState, mapping RexImportMap) {
	labelCount := map[string]int{}
	for _, item := range items {
		labelCount[item.Label]++
	}
	for i := range items {
		if labelCount[items[i].Label] > 1 && mapping.Workspaces[items[i].SourceID] == "" {
			items[i].Skip = "duplicate Herdr workspace labels; apply will refuse ambiguous destinations"
			items[i].Blocked = true
			continue
		}
		if mapping.Workspaces[items[i].SourceID] != "" {
			continue
		}
		for _, session := range state.Sessions {
			if session.Label == items[i].Label {
				items[i].Skip = "same-label Rex session exists without a source mapping; apply will refuse to adopt it"
				items[i].Blocked = true
				break
			}
		}
	}
}
