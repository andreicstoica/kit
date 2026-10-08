package liftoff

import "fmt"

func RexWorkspaceName(sessionID string) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("select a Kit workspace in Rex first")
	}
	cfg, err := LoadConfig()
	if err != nil {
		return "", err
	}
	if cfg.Settings.RexMasterSession == sessionID {
		return "master", nil
	}
	var names []string
	for name, meta := range cfg.Worktrees {
		if meta.RexID == sessionID {
			names = append(names, name)
		}
	}
	if len(names) == 1 {
		return names[0], nil
	}
	if len(names) > 1 {
		return "", fmt.Errorf("Rex session %s has multiple Kit owners", sessionID)
	}
	imports, err := LoadRexImportMap()
	if err != nil {
		return "", err
	}
	checkout := imports.Checkouts[sessionID]
	if checkout != "" {
		layout := DefaultLayout()
		if cleanPath(checkout) == cleanPath(layout.Master) {
			return "master", nil
		}
		for name, meta := range cfg.Worktrees {
			path := meta.Path
			if path == "" {
				path = layout.WorktreePath(name)
			}
			if cleanPath(path) == cleanPath(checkout) {
				names = append(names, name)
			}
		}
		if len(names) == 1 {
			return names[0], nil
		}
	}
	return "", fmt.Errorf("Rex session %s is not mapped to one Kit workspace; use Rex's sidebar or Cmd+K to select one", sessionID)
}
