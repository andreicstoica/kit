package tui

import (
	"errors"
	"fmt"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/charmbracelet/bubbles/list"
)

// This file holds the concrete standalone pickers. Each builds list items
// and hands them to the shared RunListPicker (picker.go) — the keybindings,
// quick-pick, filtering, and selection styling all live there so every
// picker behaves identically.

// editorItem is a list entry for the editor picker. The candidate type lives
// in the liftoff domain layer (internal/liftoff/workspace.go) so detection,
// launch, and UI all agree on one definition.
type editorItem struct {
	c          liftoff.EditorCandidate
	displayIdx int
}

func (e editorItem) Title() string {
	t := e.c.Name
	if e.displayIdx > 0 && e.displayIdx < 10 {
		t = StyleHi.Render(fmt.Sprintf("%d ", e.displayIdx)) + t
	}
	return t
}
func (e editorItem) Description() string { return StyleDim.Render(e.c.Desc) }
func (e editorItem) FilterValue() string { return e.c.Name }

// PickEditor opens the shared picker showing only installed editors
// (PATH binary OR app bundle).
// Returns the chosen candidate, or nil if the user pressed esc, or an error
// if no candidates are installed.
func PickEditor(editors []liftoff.EditorCandidate) (*liftoff.EditorCandidate, error) {
	var items []list.Item
	for _, e := range editors {
		if !e.Installed {
			continue
		}
		items = append(items, editorItem{c: e, displayIdx: len(items) + 1})
	}
	if len(items) == 0 {
		return nil, errors.New("no supported editor found (looked for Zed, Cursor, VS Code on PATH or in /Applications)")
	}
	chosen, ok, err := RunListPicker(ListPickerConfig{
		Title:  "kit swap — pick an editor",
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
	if a.displayIdx > 0 && a.displayIdx < 10 {
		t = StyleHi.Render(fmt.Sprintf("%d ", a.displayIdx)) + t
	}
	return t
}
func (a adoptPickItem) Description() string {
	if a.branch != "" && a.branch != a.name {
		return StyleDim.Render(a.branch + "  ·  " + a.path)
	}
	return StyleDim.Render(a.path)
}
func (a adoptPickItem) FilterValue() string { return a.name }

// PickAdoptCandidate opens a numbered picker over a slice of name/branch
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
