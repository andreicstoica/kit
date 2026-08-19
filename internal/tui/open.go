package tui

// Worktree-open dispatch lives here. Domain types and candidate lists are in
// liftoff/open_targets.go; launch primitives are in liftoff/workspace.go and
// liftoff/herdr.go. Every command that opens a worktree should call
// OpenWorktree or the Herdr helpers below — never duplicate picker lists or
// sentinel switches in cmd/.

import (
	"fmt"

	"github.com/andreicstoica/kit/internal/liftoff"
)

// HerdrConnect controls how the user lands in Herdr after the space is
// ensured. Flows that continue in the current terminal (post-design play)
// should prefer Ghostty; explicit `kit open --herdr` attaches here.
type HerdrConnect int

const (
	// HerdrConnectGhostty opens a separate Ghostty client when possible.
	HerdrConnectGhostty HerdrConnect = iota
	// HerdrConnectAttach attaches the current terminal to the Herdr session.
	HerdrConnectAttach
	// HerdrConnectNone only ensures/focuses the space and prints status.
	HerdrConnectNone
)

// OpenRequest describes how a caller wants to open a worktree. Every command
// that offers "open this worktree somewhere" should go through OpenWorktree
// so candidates, pickers, and launch behavior stay linked.
type OpenRequest struct {
	Layout liftoff.Layout
	Name   string
	Path   string

	// Explicit targets bypass the picker. When multiple are set, the first
	// matching branch below wins: EditorFlag, Herdr, WorkspaceOnly.
	EditorFlag    string
	Herdr         bool
	HerdrLayout   string
	WorkspaceOnly bool
	Detailed      bool

	// OfferSkip adds a "don't open" entry to the interactive picker.
	OfferSkip bool

	// HerdrConnect applies when Herdr is chosen via flag or picker.
	HerdrConnect HerdrConnect
}

// Validate reports conflicting explicit open targets.
func (req OpenRequest) Validate() error {
	n := 0
	if req.EditorFlag != "" {
		n++
	}
	if req.Herdr {
		n++
	}
	if req.WorkspaceOnly {
		n++
	}
	if n > 1 {
		return fmt.Errorf("choose one open target: editor (--editor), Herdr (--herdr), or Ghostty workspace (--workspace)")
	}
	return nil
}

// OpenWorktree is the single entry point for opening a worktree in an editor,
// Ghostty workspace, or Herdr space — interactively or via flags.
func OpenWorktree(req OpenRequest) (bool, error) {
	if err := req.Validate(); err != nil {
		return false, err
	}
	if req.EditorFlag != "" {
		c := liftoff.ResolveEditor(req.EditorFlag)
		if c == nil {
			return false, fmt.Errorf("editor %q not on PATH or in /Applications", req.EditorFlag)
		}
		return ExecuteOpen(req, *c)
	}
	if req.Herdr {
		return true, OpenHerdrWorktree(req.Name, req.Path, req.HerdrLayout, req.HerdrConnect)
	}
	if req.WorkspaceOnly {
		return true, openGhosttyWorkspace(req, gtabFromFlag(req.Detailed))
	}

	eds := liftoff.PickerCandidates(req.OfferSkip)
	if !req.OfferSkip {
		if sole := liftoff.LoneEditor(eds); sole != nil {
			return ExecuteOpen(req, *sole)
		}
	}
	c, err := PickOpenTarget(eds)
	if err != nil || c == nil {
		return false, err
	}
	return ExecuteOpen(req, *c)
}

// ExecuteOpen launches a chosen open target. Exported for tests and any
// caller that already resolved the destination outside the picker.
func ExecuteOpen(req OpenRequest, c liftoff.EditorCandidate) (bool, error) {
	switch c.Kind() {
	case liftoff.OpenTargetSkip:
		return false, nil
	case liftoff.OpenTargetGhosttyWorkspace:
		gl, err := PickGtabLayout(false)
		if err != nil {
			return false, err
		}
		return true, openGhosttyWorkspace(req, gl)
	case liftoff.OpenTargetHerdr:
		return true, OpenHerdrWorktree(req.Name, req.Path, req.HerdrLayout, req.HerdrConnect)
	default:
		if err := liftoff.LaunchEditor(c, req.Path); err != nil {
			return false, err
		}
		liftoff.TouchLastUsedName(req.Name)
		fmt.Printf("opened %s in %s\n", req.Path, c.Name)
		return true, nil
	}
}

// FocusHerdrRequest configures kit focus: ensure the Herdr space, optionally
// open an editor, then connect per the focus flags.
type FocusHerdrRequest struct {
	Name, Path string
	Layout     string
	Editor     string
	Ghostty    bool
	NoAttach   bool
}

// FocusHerdr is the kit focus path — Herdr-first, with optional editor and
// client wiring that OpenWorktree does not cover.
func FocusHerdr(req FocusHerdrRequest) error {
	workspace, err := EnsureHerdrWorktree(req.Name, req.Path, req.Layout)
	if err != nil {
		return err
	}
	if req.Editor != "" {
		editor := liftoff.ResolveEditor(req.Editor)
		if editor == nil {
			return fmt.Errorf("editor %q not on PATH or in /Applications", req.Editor)
		}
		if err := liftoff.LaunchEditor(*editor, req.Path); err != nil {
			return err
		}
	}
	return ConnectHerdr(req.Name, workspace, herdrConnectForFocus(req.Ghostty, req.NoAttach))
}

func herdrConnectForFocus(ghostty, noAttach bool) HerdrConnect {
	if ghostty {
		return HerdrConnectGhostty
	}
	if noAttach {
		return HerdrConnectNone
	}
	return HerdrConnectAttach
}

func openGhosttyWorkspace(req OpenRequest, gl liftoff.GtabLayout) error {
	if err := liftoff.OpenWorkspace(req.Layout, req.Name, req.Path, gl); err != nil {
		return err
	}
	fmt.Printf("opened %s workspace (ghostty)\n", req.Name)
	return nil
}

// ResolveHerdrLayout keeps reconnects quiet while making the first open
// explicit. A requested layout always wins; otherwise only a worktree with
// no existing Herdr space gets the simple/detailed picker.
func ResolveHerdrLayout(name, path, requested string) (string, error) {
	if requested != "" {
		return requested, nil
	}
	exists, err := liftoff.HerdrWorkspaceExists(name, path)
	if err != nil {
		return "", err
	}
	if exists {
		return "", nil
	}
	return PickHerdrLayout()
}

// EnsureHerdrWorktree creates or reuses the Herdr space without connecting a
// client. Use OpenHerdrWorktree when the caller is ready to land the user.
func EnsureHerdrWorktree(name, path, requestedLayout string) (liftoff.HerdrWorkspace, error) {
	chosenLayout, err := ResolveHerdrLayout(name, path, requestedLayout)
	if err != nil {
		return liftoff.HerdrWorkspace{}, err
	}
	return liftoff.OpenHerdr(name, path, chosenLayout)
}

// OpenHerdrWorktree ensures the Herdr space exists, then connects per mode.
func OpenHerdrWorktree(name, path, requestedLayout string, connect HerdrConnect) error {
	workspace, err := EnsureHerdrWorktree(name, path, requestedLayout)
	if err != nil {
		return err
	}
	return ConnectHerdr(name, workspace, connect)
}

// ConnectHerdr lands the user in an already-focused Herdr workspace.
func ConnectHerdr(name string, workspace liftoff.HerdrWorkspace, connect HerdrConnect) error {
	switch connect {
	case HerdrConnectAttach:
		fmt.Printf("opened %s in Herdr (%s)\n", name, workspace.WorkspaceID)
		return liftoff.AttachHerdr()
	case HerdrConnectNone:
		fmt.Printf("focused %s in Herdr (%s)\n", name, workspace.WorkspaceID)
		return nil
	default:
		if err := liftoff.LaunchHerdrInGhostty(); err != nil {
			fmt.Printf("opened %s in Herdr (%s); attach with `kit open --herdr %s`\n",
				name, workspace.WorkspaceID, name)
			return nil
		}
		fmt.Printf("opened %s in Herdr (%s)\n", name, workspace.WorkspaceID)
		return nil
	}
}

func gtabFromFlag(detailed bool) liftoff.GtabLayout {
	if detailed {
		return liftoff.GtabDetailed
	}
	return liftoff.GtabSimple
}
