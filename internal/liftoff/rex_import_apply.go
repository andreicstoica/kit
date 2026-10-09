package liftoff

import (
	"encoding/json"
	"fmt"
	"strings"
)

const rexImportWindowLua = `local a=rex.args
local w,err=rex.session.new_window{layout=rex.layout.block{flavor="com.superlogical.terminal.shell",options={cwd=a.cwd}},focus=false}
if err then error(err,0) end
local id=w.window_id or w.id
local _,label_err=rex.session.set_window_label{window_id=id,label=a.marker}
return {window_id=id,block_ids=w.block_ids or {},marker_error=label_err}`

const rexImportSplitLua = `local a=rex.args
local s,err=rex.session.new_split{block_id=a.anchor,direction="horizontal",side="after",focus=false,layout=rex.layout.block{flavor="com.superlogical.terminal.shell",options={cwd=a.cwd}}}
if err then error(err,0) end
local id=s.block_ids and s.block_ids[1]
if not id then error("split returned no block id",0) end
return {block_id=id}`

type rexCreatedWindow struct {
	WindowID string   `json:"window_id"`
	BlockIDs []string `json:"block_ids"`
}

// ApplyRexImport reconstructs only shell terminals and persists each returned
// ID before continuing. A retry can therefore safely resume after partial work.
func ApplyRexImport(source HerdrState, mapping *RexImportMap) error {
	return withWorkspaceLock("rex:import", func() error {
		latest, err := LoadRexImportMap()
		if err != nil {
			return err
		}
		// Preserve only caller-provided entries absent from durable storage. The
		// durable map wins on conflicts so a stale caller cannot overwrite IDs.
		for k, v := range mapping.Workspaces {
			if _, ok := latest.Workspaces[k]; !ok {
				latest.Workspaces[k] = v
			}
		}
		for k, v := range mapping.Tabs {
			if _, ok := latest.Tabs[k]; !ok {
				latest.Tabs[k] = v
			}
		}
		for k, v := range mapping.Panes {
			if _, ok := latest.Panes[k]; !ok {
				latest.Panes[k] = v
			}
		}
		state, err := readRexStructure("")
		if err != nil {
			return err
		}
		*mapping = latest
		return applyRexImport(source, mapping, state)
	})
}

func applyRexImport(source HerdrState, mapping *RexImportMap, existing RexState) error {
	if mapping.Workspaces == nil {
		mapping.Workspaces = map[string]string{}
	}
	if mapping.Tabs == nil {
		mapping.Tabs = map[string]string{}
	}
	if mapping.Panes == nil {
		mapping.Panes = map[string]string{}
	}
	if mapping.Checkouts == nil {
		mapping.Checkouts = map[string]string{}
	}
	if len(source.Workspaces) == 0 {
		return fmt.Errorf("no Herdr workspaces to import")
	}
	for _, ws := range source.Workspaces {
		if ws.WorkspaceID == "" {
			return fmt.Errorf("Herdr workspace %q has no stable source ID", ws.Label)
		}
		for _, tab := range importTabs(source, ws.WorkspaceID) {
			if tab.TabID == "" {
				return fmt.Errorf("Herdr tab %q in workspace %q has no stable source ID", tab.Label, ws.Label)
			}
			for _, pane := range importPanes(source, tab.TabID) {
				if pane.PaneID == "" {
					return fmt.Errorf("Herdr tab %q contains a pane with no stable source ID", tab.Label)
				}
			}
		}
	}
	for _, ws := range source.Workspaces {
		importWS := ws
		if importWS.Worktree == nil || !dirExists(importWS.Worktree.CheckoutPath) {
			if root := workspaceImportRoot(source, ws); root != "" {
				importWS.Worktree = &HerdrWorktreeRef{CheckoutPath: root}
			}
		}
		if !dirExists(wsImportCWD(importWS, nil)) {
			return fmt.Errorf("workspace %q has no available checkout directory", ws.Label)
		}
		mapping.Checkouts[ws.WorkspaceID] = cleanPath(importWS.Worktree.CheckoutPath)
		destID := mapping.Workspaces[ws.WorkspaceID]
		dest := rexSessionByID(existing, destID)
		if dest == nil {
			if destID != "" {
				return fmt.Errorf("mapped Rex session %q for Herdr workspace %q is missing; refusing to recreate automatically", destID, ws.WorkspaceID)
			}
			for _, s := range existing.Sessions {
				if s.Label == ws.Label {
					return fmt.Errorf("Rex already has unrelated session labeled %q; refusing to adopt by label", ws.Label)
				}
			}
			cwd := workspaceInitialCWD(source, importWS)
			if !dirExists(cwd) {
				return fmt.Errorf("workspace %q has no available checkout directory", ws.Label)
			}
			out, err := runRex("new", ws.Label, "--cwd", cwd, "--window", "shell", "--json")
			if err != nil {
				return err
			}
			var created struct {
				SessionID string `json:"session_id"`
			}
			if err := json.Unmarshal([]byte(out), &created); err != nil || created.SessionID == "" {
				return fmt.Errorf("parse Rex session creation response: missing session_id")
			}
			destID = created.SessionID
			mapping.Workspaces[ws.WorkspaceID] = destID
			if err := SaveRexImportMap(*mapping); err != nil {
				return err
			}
			if err := saveImportedKitMapping(importWS, destID); err != nil {
				return err
			}
			existing, err = readRexStructure("")
			if err != nil {
				return fmt.Errorf("read newly created Rex session: %w", err)
			}
			dest = rexSessionByID(existing, destID)
			if dest == nil {
				return fmt.Errorf("Rex created session %q but it was not present in state", destID)
			}
		}
		tabs := importTabs(source, ws.WorkspaceID)
		for tabIndex, tab := range tabs {
			paneList := importPanes(source, tab.TabID)
			if len(paneList) == 0 {
				paneList = []HerdrPane{{PaneID: "kit-empty-tab:" + tab.TabID, WorkspaceID: ws.WorkspaceID}}
			}
			cwdValues := make([]string, len(paneList))
			for i := range paneList {
				cwdValues[i] = wsImportCWD(importWS, paneList[i].CWD)
			}
			mappedTabID := mapping.Tabs[tab.TabID]
			mappedWindow := rexWindowByID(*dest, mappedTabID)
			if mappedTabID != "" && mappedWindow == nil {
				return fmt.Errorf("Rex window mapping for Herdr tab %s is stale; refusing to recreate it", tab.TabID)
			}
			if mappedWindow != nil {
				for _, p := range paneList {
					if target := mapping.Panes[p.PaneID]; target != "" && !rexBlockExists(*mappedWindow, target) {
						return fmt.Errorf("Rex block mapping for Herdr pane %s is stale; refusing to recreate it", p.PaneID)
					}
				}
				// An unmapped actual block may be the result of a create whose response
				// was lost. Never guess its source identity from block order.
				for _, block := range mappedWindow.Blocks {
					known := false
					for _, p := range paneList {
						if mapping.Panes[p.PaneID] == block.BlockID {
							known = true
							break
						}
					}
					if !known {
						return fmt.Errorf("Rex tab %q has an unmapped block %s; refusing to infer its source identity", tab.Label, block.BlockID)
					}
				}
				for i, p := range paneList {
					if p.PaneID == "" || mapping.Panes[p.PaneID] != "" {
						continue
					}
					anchor := firstBlockID(*mappedWindow)
					if i > 0 && mapping.Panes[paneList[0].PaneID] != "" {
						anchor = mapping.Panes[paneList[0].PaneID]
					}
					payload := struct {
						Anchor string `json:"anchor"`
						CWD    string `json:"cwd"`
					}{anchor, cwdValues[i]}
					args, _ := json.Marshal(payload)
					out, err := runRex("do", "-s", destID, "-e", rexImportSplitLua, "--args", string(args))
					if err != nil {
						return fmt.Errorf("create shell split for Herdr pane %s: %w", p.PaneID, err)
					}
					var split struct {
						BlockID string `json:"block_id"`
					}
					if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &split); err != nil || split.BlockID == "" {
						return fmt.Errorf("parse split response for Herdr pane %s", p.PaneID)
					}
					mapping.Panes[p.PaneID] = split.BlockID
					if err := SaveRexImportMap(*mapping); err != nil {
						return err
					}
				}
				if mappedWindow.Label != tab.Label {
					if err := setRexWindowLabel(destID, mappedWindow.WindowID, tab.Label); err != nil {
						return err
					}
				}
				if err := SaveRexImportMap(*mapping); err != nil {
					return err
				}
				continue
			}
			// The initial shell comes from `rex new`; import it as the first source
			// tab, then create all other windows using Rex's documented Lua API.
			if tabIndex == 0 && len(dest.Windows) == 0 {
				return fmt.Errorf("Rex session %q has no initial window in its state; retry after state refresh", destID)
			}
			if tabIndex == 0 && mapping.Tabs[tab.TabID] == "" {
				window := dest.Windows[0]
				mapping.Tabs[tab.TabID] = window.WindowID
				if err := SaveRexImportMap(*mapping); err != nil {
					return err
				}
				anchor := firstBlockID(window)
				if anchor == "" && len(paneList) > 1 {
					return fmt.Errorf("Rex initial window has no shell block")
				}
				if len(paneList) > 0 && paneList[0].PaneID != "" {
					mapping.Panes[paneList[0].PaneID] = anchor
					if err := SaveRexImportMap(*mapping); err != nil {
						return err
					}
				}
				if len(paneList) > 1 {
					for i := 1; i < len(paneList); i++ {
						payload := struct {
							Anchor string `json:"anchor"`
							CWD    string `json:"cwd"`
						}{anchor, cwdValues[i]}
						args, _ := json.Marshal(payload)
						out, err := runRex("do", "-s", destID, "-e", rexImportSplitLua, "--args", string(args))
						if err != nil {
							return fmt.Errorf("create shell split for Herdr pane %s: %w", paneList[i].PaneID, err)
						}
						var split struct {
							BlockID string `json:"block_id"`
						}
						if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &split); err != nil || split.BlockID == "" {
							return fmt.Errorf("parse split response for Herdr pane %s", paneList[i].PaneID)
						}
						mapping.Panes[paneList[i].PaneID] = split.BlockID
						if err := SaveRexImportMap(*mapping); err != nil {
							return err
						}
					}
				}
				if err := SaveRexImportMap(*mapping); err != nil {
					return err
				}
				if err := setRexWindowLabel(destID, window.WindowID, tab.Label); err != nil {
					return err
				}
				continue
			}
			for _, existingWindow := range dest.Windows {
				if existingWindow.Label == tab.Label || existingWindow.Label == "kit-import:"+tab.TabID {
					return fmt.Errorf("Rex tab %q exists without a source mapping; refusing to adopt or duplicate it", tab.Label)
				}
			}
			payload := struct {
				CWD    string `json:"cwd"`
				Marker string `json:"marker"`
			}{CWD: cwdValues[0], Marker: "kit-import:" + tab.TabID}
			args, _ := json.Marshal(payload)
			out, err := runRex("do", "-s", destID, "-e", rexImportWindowLua, "--args", string(args))
			if err != nil {
				return fmt.Errorf("create Rex tab %q: %w", tab.Label, err)
			}
			var created rexCreatedWindow
			if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &created); err != nil || created.WindowID == "" {
				return fmt.Errorf("parse Rex tab %q creation response", tab.Label)
			}
			mapping.Tabs[tab.TabID] = created.WindowID
			if err := SaveRexImportMap(*mapping); err != nil {
				return err
			}
			if len(paneList) > 0 && paneList[0].PaneID != "" {
				if len(created.BlockIDs) != 1 || created.BlockIDs[0] == "" {
					return fmt.Errorf("Rex tab %q did not return its root block ID", tab.Label)
				}
				mapping.Panes[paneList[0].PaneID] = created.BlockIDs[0]
			}
			if err := SaveRexImportMap(*mapping); err != nil {
				return err
			}
			if err := setRexWindowLabel(destID, created.WindowID, tab.Label); err != nil {
				return err
			}
			anchor := ""
			if len(created.BlockIDs) > 0 {
				anchor = created.BlockIDs[0]
			}
			for i := 1; i < len(paneList); i++ {
				payload := struct {
					Anchor string `json:"anchor"`
					CWD    string `json:"cwd"`
				}{anchor, cwdValues[i]}
				args, _ := json.Marshal(payload)
				out, err := runRex("do", "-s", destID, "-e", rexImportSplitLua, "--args", string(args))
				if err != nil {
					return fmt.Errorf("create shell split for Herdr pane %s: %w", paneList[i].PaneID, err)
				}
				var split struct {
					BlockID string `json:"block_id"`
				}
				if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &split); err != nil || split.BlockID == "" {
					return fmt.Errorf("parse split response for Herdr pane %s", paneList[i].PaneID)
				}
				mapping.Panes[paneList[i].PaneID] = split.BlockID
				if err := SaveRexImportMap(*mapping); err != nil {
					return err
				}
			}
		}
		var rootWindowID string
		if len(tabs) > 0 {
			rootWindowID = mapping.Tabs[tabs[0].TabID]
		}
		if err := saveImportedKitMapping(importWS, destID, rootWindowID); err != nil {
			return err
		}
	}
	return nil
}

func rexSessionByID(s RexState, id string) *RexSession {
	if id == "" {
		return nil
	}
	for i := range s.Sessions {
		if s.Sessions[i].SessionID == id {
			return &s.Sessions[i]
		}
	}
	return nil
}

func rexWindowByID(s RexSession, id string) *RexWindow {
	if id == "" {
		return nil
	}
	for i := range s.Windows {
		if s.Windows[i].WindowID == id {
			return &s.Windows[i]
		}
	}
	return nil
}

func rexBlockExists(w RexWindow, id string) bool {
	for _, b := range w.Blocks {
		if b.BlockID == id {
			return true
		}
	}
	return false
}

func setRexWindowLabel(sessionID, windowID, label string) error {
	_, err := runRex("do", "-s", sessionID, "-e", fmt.Sprintf(`local _,e=rex.session.set_window_label{window_id=%q,label=%q}; if e then error(e,0) end; return true`, windowID, label))
	return err
}

// saveImportedKitMapping updates Kit's runtime IDs only through stable Herdr IDs
// or an exact, existing checkout path. Display labels are never identifiers.
func saveImportedKitMapping(ws HerdrWorkspace, rexID string, rootWindowID ...string) error {
	layout := DefaultLayout()
	return WithConfigLock(func(c *Config) error {
		root := ""
		if ws.Worktree != nil && dirExists(ws.Worktree.CheckoutPath) {
			root = cleanPath(ws.Worktree.CheckoutPath)
		}
		masterDir := ""
		if dirExists(layout.Master) {
			masterDir = cleanPath(layout.Master)
		}
		if root != "" && root == masterDir {
			c.Settings.RexMasterSession = rexID
			if len(rootWindowID) > 0 && rootWindowID[0] != "" {
				c.Settings.RexMasterShellWindow = rootWindowID[0]
			}
		}
		matchedHerdrID := false
		for _, meta := range c.Worktrees {
			if meta.HerdrID == ws.WorkspaceID {
				matchedHerdrID = true
				break
			}
		}
		for name, meta := range c.Worktrees {
			pathMatch := root != "" && meta.Path != "" && dirExists(meta.Path) && cleanPath(meta.Path) == root
			if meta.HerdrID == ws.WorkspaceID || (!matchedHerdrID && meta.HerdrID == "" && pathMatch) {
				meta.RexID = rexID
				if len(rootWindowID) > 0 && rootWindowID[0] != "" {
					meta.RexShellWindow = rootWindowID[0]
				}
				if meta.RexLayout == "" {
					meta.RexLayout = meta.HerdrLayout
				}
				c.Worktrees[name] = meta
			}
		}
		return nil
	})
}

func firstBlockID(w RexWindow) string {
	if len(w.Blocks) == 0 {
		return ""
	}
	return w.Blocks[0].BlockID
}

func wsImportCWD(ws HerdrWorkspace, paneCWD *string) string {
	root := ""
	if ws.Worktree != nil && dirExists(ws.Worktree.CheckoutPath) {
		root = cleanPath(ws.Worktree.CheckoutPath)
	}
	if paneCWD != nil && dirExists(*paneCWD) {
		cwd := cleanPath(*paneCWD)
		if root != "" && isPathInside(cwd, root) {
			return cwd
		}
	}
	return root
}

func workspaceInitialCWD(source HerdrState, ws HerdrWorkspace) string {
	root := ""
	if ws.Worktree != nil && dirExists(ws.Worktree.CheckoutPath) {
		root = cleanPath(ws.Worktree.CheckoutPath)
	}
	for _, pane := range source.Panes {
		if pane.WorkspaceID == ws.WorkspaceID && pane.CWD != nil && dirExists(*pane.CWD) {
			cwd := cleanPath(*pane.CWD)
			if root == "" || isPathInside(cwd, root) {
				return cwd
			}
		}
	}
	return root
}

func importTabs(s HerdrState, id string) []HerdrTab {
	var tabs []HerdrTab
	for _, t := range s.Tabs {
		if t.WorkspaceID == id {
			tabs = append(tabs, t)
		}
	}
	return tabs
}

func importPanes(s HerdrState, tabID string) []HerdrPane {
	var panes []HerdrPane
	for _, p := range s.Panes {
		if p.TabID == tabID {
			panes = append(panes, p)
		}
	}
	return panes
}
