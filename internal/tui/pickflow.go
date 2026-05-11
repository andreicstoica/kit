package tui

import (
	"errors"
	"fmt"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// pickModel is a simple Bubble Tea picker that returns a worktree name.
type pickModel struct {
	list   list.Model
	chosen string
	cancel bool
}

func (m *pickModel) Init() tea.Cmd { return nil }

func (m *pickModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height-2)
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancel = true
			return m, tea.Quit
		case "enter":
			if it, ok := m.list.SelectedItem().(playWtItem); ok {
				m.chosen = it.name
				return m, tea.Quit
			}
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			// Numeric quick-pick: jump straight to that visible item.
			// Respect the list's filtered view when active.
			idx := int(msg.String()[0] - '0' - 1)
			items := m.list.VisibleItems()
			if idx >= 0 && idx < len(items) {
				if it, ok := items[idx].(playWtItem); ok {
					m.chosen = it.name
					return m, tea.Quit
				}
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *pickModel) View() string { return m.list.View() }

// EditorCandidate describes one possible editor + its install state.
//
// On macOS, an editor may be installed as a `.app` bundle without a
// PATH binary. The cmd/swap.go detection layer handles the bundle vs
// PATH distinction; this struct just carries the resolved Launch field.
type EditorCandidate struct {
	Name      string
	Binary    string // CLI binary name (preferred when on PATH)
	App       string // .app bundle name (e.g. "Zed.app") for `open -a`
	Desc      string
	Installed bool
	UseOpen   bool   // true when launch is via `open -a App` not `binary`
}

// editorItem is a list entry for the editor picker.
type editorItem struct {
	c          EditorCandidate
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

type pickEditorModel struct {
	list   list.Model
	chosen *EditorCandidate
	cancel bool
}

func (m *pickEditorModel) Init() tea.Cmd { return nil }
func (m *pickEditorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height-2)
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancel = true
			return m, tea.Quit
		case "enter":
			if it, ok := m.list.SelectedItem().(editorItem); ok {
				c := it.c
				m.chosen = &c
				return m, tea.Quit
			}
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			idx := int(msg.String()[0] - '0' - 1)
			items := m.list.VisibleItems()
			if idx >= 0 && idx < len(items) {
				if it, ok := items[idx].(editorItem); ok {
					c := it.c
					m.chosen = &c
					return m, tea.Quit
				}
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}
func (m *pickEditorModel) View() string { return m.list.View() }

// PickEditor opens a Bubble Tea picker showing only installed editors
// (PATH binary OR app bundle).
// Returns the chosen candidate, or nil if the user pressed esc, or an error
// if no candidates are installed.
func PickEditor(editors []EditorCandidate) (*EditorCandidate, error) {
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
	// Always show the picker, even for a single item — user asked for an
	// explicit choice rather than implicit auto-pick.
	dlg := list.NewDefaultDelegate()
	dlg.Styles.SelectedTitle = dlg.Styles.SelectedTitle.Foreground(colorAccent).BorderForeground(colorAccent)
	dlg.Styles.SelectedDesc = dlg.Styles.SelectedDesc.Foreground(colorAccent).BorderForeground(colorAccent)
	l := list.New(items, dlg, 0, 0)
	l.Title = "kit swap — pick an editor"
	l.Styles.Title = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	l.SetShowHelp(true)
	l.SetFilteringEnabled(false)
	m := &pickEditorModel{list: l}
	final, runErr := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if runErr != nil {
		return nil, runErr
	}
	pm, ok := final.(*pickEditorModel)
	if !ok || pm.cancel {
		return nil, nil
	}
	return pm.chosen, nil
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

type adoptPickModel struct {
	list   list.Model
	chosen string
	cancel bool
}

func (m *adoptPickModel) Init() tea.Cmd { return nil }

func (m *adoptPickModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height-2)
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancel = true
			return m, tea.Quit
		case "enter":
			if it, ok := m.list.SelectedItem().(adoptPickItem); ok {
				m.chosen = it.name
				return m, tea.Quit
			}
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			idx := int(msg.String()[0] - '0' - 1)
			items := m.list.VisibleItems()
			if idx >= 0 && idx < len(items) {
				if it, ok := items[idx].(adoptPickItem); ok {
					m.chosen = it.name
					return m, tea.Quit
				}
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}
func (m *adoptPickModel) View() string { return m.list.View() }

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
	dlg := list.NewDefaultDelegate()
	dlg.Styles.SelectedTitle = dlg.Styles.SelectedTitle.Foreground(colorAccent).BorderForeground(colorAccent)
	dlg.Styles.SelectedDesc = dlg.Styles.SelectedDesc.Foreground(colorAccent).BorderForeground(colorAccent)
	l := list.New(items, dlg, 0, 0)
	l.Title = "kit adopt — pick a worktree"
	l.Styles.Title = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	l.SetShowHelp(true)
	l.SetFilteringEnabled(true)
	m := &adoptPickModel{list: l}
	final, runErr := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if runErr != nil {
		return "", runErr
	}
	pm, ok := final.(*adoptPickModel)
	if !ok || pm.cancel {
		return "", nil
	}
	return pm.chosen, nil
}

// PickWorktree opens a Bubble Tea picker listing every worktree that has a
// directory on disk. Returns the chosen name, or "" with no error if the
// user pressed esc.
func PickWorktree(layout liftoff.Layout, prompt string) (string, error) {
	items, err := collectPlayWtItems(layout, nil)
	if err != nil {
		return "", err
	}
	if len(items) == 0 {
		return "", errors.New("no worktrees found — run `kit design` first")
	}
	dlg := list.NewDefaultDelegate()
	dlg.Styles.SelectedTitle = dlg.Styles.SelectedTitle.Foreground(colorAccent).BorderForeground(colorAccent)
	dlg.Styles.SelectedDesc = dlg.Styles.SelectedDesc.Foreground(colorAccent).BorderForeground(colorAccent)
	l := list.New(items, dlg, 0, 0)
	l.Title = prompt
	l.Styles.Title = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	l.SetShowHelp(true)
	l.SetFilteringEnabled(true)
	m := &pickModel{list: l}
	final, runErr := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if runErr != nil {
		return "", runErr
	}
	pm, ok := final.(*pickModel)
	if !ok || pm.cancel {
		return "", nil
	}
	if pm.chosen == "" {
		return "", fmt.Errorf("no worktree selected")
	}
	return pm.chosen, nil
}
