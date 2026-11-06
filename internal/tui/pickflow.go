package tui

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/list"
	"github.com/andreicstoica/kit/internal/liftoff"
)

// This file holds the concrete standalone pickers. Each builds list items
// and hands them to the shared RunListPicker (picker.go) — the keybindings,
// quick-pick, filtering, and selection styling all live there so every
// picker behaves identically.

// editorItem is a list entry for the editor picker. The candidate type lives
// in the liftoff domain layer (internal/liftoff/open_targets.go) so detection,
// launch, and UI all agree on one definition.
type editorItem struct {
	c          liftoff.EditorCandidate
	displayIdx int
}

func (e editorItem) Title() string {
	t := e.c.Name
	t = shortcutTitle(t, e.displayIdx)
	return t
}
func (e editorItem) Description() string { return StyleDim.Render(e.c.Desc) }
func (e editorItem) FilterValue() string { return e.c.Name }
func (e editorItem) shortcut() string    { return shortcutLabel(e.displayIdx) }

// PickOpenTarget opens the shared picker for worktree-open destinations.
func PickOpenTarget(candidates []liftoff.EditorCandidate) (*liftoff.EditorCandidate, error) {
	return pickEditor(candidates, "kit open — pick where to open")
}

func pickEditor(editors []liftoff.EditorCandidate, title string) (*liftoff.EditorCandidate, error) {
	var items []list.Item
	for _, e := range editors {
		if !e.Installed {
			continue
		}
		items = append(items, editorItem{c: e, displayIdx: len(items) + 1})
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no supported editor found (looked for %s on PATH or in /Applications)", strings.Join(liftoff.EditorNames(), ", "))
	}
	chosen, ok, err := RunListPicker(ListPickerConfig{
		Title:  title,
		Items:  items,
		Filter: false,
	})
	if err != nil || !ok {
		return nil, err
	}
	c := chosen.(editorItem).c
	return &c, nil
}

// adoptPickItem renders one adopt candidate in the bubble list.
type adoptPickItem struct {
	name       string
	branch     string
	path       string
	displayIdx int
}

func (a adoptPickItem) Title() string {
	t := a.name
	t = shortcutTitle(t, a.displayIdx)
	return t
}
func (a adoptPickItem) Description() string {
	if a.branch != "" && a.branch != a.name {
		return StyleDim.Render(a.branch + "  ·  " + a.path)
	}
	return StyleDim.Render(a.path)
}
func (a adoptPickItem) FilterValue() string { return a.name }
func (a adoptPickItem) shortcut() string    { return shortcutLabel(a.displayIdx) }

// PickAdoptCandidate opens a labeled picker over a slice of name/branch
// pairs. Returns the chosen name, "" on cancel.
func PickAdoptCandidate(cands []liftoff.AdoptCandidate) (string, error) {
	if len(cands) == 0 {
		return "", errors.New("no adoptable worktrees")
	}
	items := make([]list.Item, 0, len(cands))
	for i, c := range cands {
		items = append(items, adoptPickItem{
			name:       c.Name,
			branch:     c.Branch,
			path:       c.Path,
			displayIdx: i + 1,
		})
	}
	chosen, ok, err := RunListPicker(ListPickerConfig{
		Title:  "kit adopt — pick a worktree",
		Items:  items,
		Filter: true,
	})
	if err != nil || !ok {
		return "", err
	}
	return chosen.(adoptPickItem).name, nil
}

// PickWorktree opens a picker listing every worktree that has a directory on
// disk. Returns the chosen name, or "" with no error if the user cancelled.
func PickWorktree(layout liftoff.Layout, prompt string) (string, error) {
	items, err := collectPlayWtItems(layout, nil)
	if err != nil {
		return "", err
	}
	if len(items) == 0 {
		return "", errors.New("no worktrees found — run `kit design` first")
	}
	chosen, ok, err := RunListPicker(ListPickerConfig{
		Title:  prompt,
		Items:  items,
		Filter: true,
	})
	if err != nil || !ok {
		return "", err
	}
	return chosen.(playWtItem).name, nil
}
