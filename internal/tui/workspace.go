package tui

import (
	"fmt"

	"github.com/andreicstoica/kit/internal/liftoff"
)

// OpenRequest describes how a caller wants to open a worktree. It is the one
// entry point every command uses, so "open this worktree" behaves the same
// in `kit swap`, `kit design`, and anywhere else.
type OpenRequest struct {
	Layout liftoff.Layout
	Name   string
	Path   string

	// EditorFlag, when non-empty, opens that editor directly (no picker).
	EditorFlag string
	// WorkspaceOnly skips the editor entirely and launches the Ghostty
	// workspace using Detailed to choose the layout.
	WorkspaceOnly bool
	// Detailed selects the 5-tab Ghostty layout for WorkspaceOnly.
	Detailed bool
	// OfferSkip adds a "don't open" entry to the interactive picker (used by
	// post-design where opening is optional).
	OfferSkip bool
}

// OpenWorktree opens a worktree the same way everywhere: an explicit editor,
// a forced Ghostty workspace, or — by default — an interactive picker that
// lists installed editors plus the Ghostty workspace (and optionally a skip
// entry). Returns true when something was opened, false when the user
// cancelled or chose to skip.
func OpenWorktree(req OpenRequest) (bool, error) {
	// Explicit editor via flag.
	if req.EditorFlag != "" {
		c := liftoff.ResolveEditor(req.EditorFlag)
		if c == nil {
			return false, fmt.Errorf("editor %q not on PATH or in /Applications", req.EditorFlag)
		}
		return openCandidate(req, *c)
	}

	// Forced Ghostty workspace.
	if req.WorkspaceOnly {
		return true, openWorkspaceLayout(req, gtabFromFlag(req.Detailed))
	}

	// Interactive: build the unified candidate list.
	eds := liftoff.InstalledEditors()
	if req.OfferSkip {
		eds = append(eds, liftoff.EditorCandidate{
			Name:      "Skip — don't open",
			Binary:    liftoff.SkipSentinel,
			Desc:      "leave the worktree closed for now",
			Installed: true,
		})
	}
	// One real editor and nothing else → skip the picker. Suppressed when a
	// skip entry was requested, so that escape hatch stays reachable.
	if !req.OfferSkip {
		if sole := liftoff.LoneEditor(eds); sole != nil {
			return openCandidate(req, *sole)
		}
	}
	c, err := PickEditor(eds)
	if err != nil || c == nil {
		return false, err
	}
	return openCandidate(req, *c)
}

// openCandidate dispatches a chosen candidate to the right launch path.
func openCandidate(req OpenRequest, c liftoff.EditorCandidate) (bool, error) {
	switch c.Binary {
	case liftoff.SkipSentinel:
		return false, nil
	case liftoff.WorkspaceSentinel:
		gl, err := PickGtabLayout(false)
		if err != nil {
			return false, err
		}
		return true, openWorkspaceLayout(req, gl)
	default:
		if err := liftoff.LaunchEditor(c, req.Path); err != nil {
			return false, err
		}
		liftoff.TouchLastUsedName(req.Name)
		fmt.Printf("opened %s in %s\n", req.Path, c.Name)
		return true, nil
	}
}

func openWorkspaceLayout(req OpenRequest, gl liftoff.GtabLayout) error {
	if err := liftoff.OpenWorkspace(req.Layout, req.Name, req.Path, gl); err != nil {
		return err
	}
	fmt.Printf("opened %s workspace (ghostty)\n", req.Name)
	return nil
}

func gtabFromFlag(detailed bool) liftoff.GtabLayout {
	if detailed {
		return liftoff.GtabDetailed
	}
	return liftoff.GtabSimple
}
