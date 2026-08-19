package liftoff

// Worktree-open taxonomy: EditorCandidate doubles as the unified picker row
// type. Sentinel Binary values mark non-editor destinations; Kind() is the
// dispatch key used by internal/tui/open.go. Commands should never branch on
// sentinel strings directly — use Kind() or go through OpenWorktree.

// WorkspaceSentinel marks the legacy Ghostty gtab workspace target.
const WorkspaceSentinel = "__workspace__"

// SkipSentinel marks a "don't open anything" picker entry.
const SkipSentinel = "__skip__"

// HerdrSentinel marks the persistent Herdr space target.
const HerdrSentinel = "__herdr__"

// OpenTargetKind classifies a worktree-open destination.
type OpenTargetKind int

const (
	OpenTargetEditor OpenTargetKind = iota
	OpenTargetGhosttyWorkspace
	OpenTargetHerdr
	OpenTargetSkip
)

// Kind reports what launching this candidate will do.
func (c EditorCandidate) Kind() OpenTargetKind {
	switch c.Binary {
	case WorkspaceSentinel:
		return OpenTargetGhosttyWorkspace
	case HerdrSentinel:
		return OpenTargetHerdr
	case SkipSentinel:
		return OpenTargetSkip
	default:
		return OpenTargetEditor
	}
}

// HerdrCandidate is the picker entry for the persistent Herdr space.
func HerdrCandidate() EditorCandidate {
	return EditorCandidate{
		Name:      "Herdr space",
		Binary:    HerdrSentinel,
		Desc:      "persistent terminal session (reachable from your phone)",
		Installed: true,
	}
}

// WorkspaceCandidate is the picker entry for the legacy Ghostty gtab workspace.
func WorkspaceCandidate() EditorCandidate {
	return EditorCandidate{
		Name:      "Ghostty (pick layout next)",
		Binary:    WorkspaceSentinel,
		App:       "Ghostty.app",
		Desc:      "dev workspace — simple (2 tabs) or detailed (5 tabs)",
		Installed: true,
	}
}

// SkipCandidate is the picker entry for declining to open anything.
func SkipCandidate() EditorCandidate {
	return EditorCandidate{
		Name:      "Skip — don't open",
		Binary:    SkipSentinel,
		Desc:      "leave the worktree closed for now",
		Installed: true,
	}
}

// OpenCandidates returns the unified worktree-open picker list: installed
// editors, the Ghostty workspace when present, and the Herdr space when the
// CLI is available. Herdr is last because editors are the common case.
func OpenCandidates() []EditorCandidate {
	out := InstalledEditors()
	if HerdrAvailable() {
		out = append(out, HerdrCandidate())
	}
	return out
}

// PickerCandidates is OpenCandidates plus optional flow-specific entries.
func PickerCandidates(offerSkip bool) []EditorCandidate {
	out := OpenCandidates()
	if offerSkip {
		out = append(out, SkipCandidate())
	}
	return out
}

// LoneEditor returns the single installed editor when no picker is needed.
// Returns nil when there are zero editors, two or more editors, or any number
// plus a non-editor open target (Ghostty workspace or Herdr), which are
// distinct intents that always warrant the picker. Skip is ignored.
func LoneEditor(eds []EditorCandidate) *EditorCandidate {
	var editors []EditorCandidate
	hasNonEditor := false
	for _, e := range eds {
		switch e.Kind() {
		case OpenTargetGhosttyWorkspace, OpenTargetHerdr:
			hasNonEditor = true
		case OpenTargetSkip:
			// ignore
		default:
			editors = append(editors, e)
		}
	}
	if len(editors) == 1 && !hasNonEditor {
		c := editors[0]
		return &c
	}
	return nil
}
