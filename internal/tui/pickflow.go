package tui

import (
	"errors"
	"fmt"
	"sort"

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
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *pickModel) View() string { return m.list.View() }

// EditorCandidate describes one possible editor + its install state.
type EditorCandidate struct {
	Name      string
	Binary    string
	Desc      string
	Installed bool
}

// editorItem is a list entry for the editor picker.
type editorItem struct {
	c EditorCandidate
}

func (e editorItem) Title() string       { return e.c.Name }
func (e editorItem) Description() string { return StyleDim.Render(e.c.Desc) }
func (e editorItem) FilterValue() string { return e.c.Name }

type pickEditorModel struct {
	list   list.Model
	chosen string
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
				m.chosen = it.c.Binary
				return m, tea.Quit
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}
func (m *pickEditorModel) View() string { return m.list.View() }

// PickEditor opens a Bubble Tea picker showing only editors found on PATH.
// Returns the chosen binary name, or "" if the user pressed esc, or an error
// if no candidates are installed.
func PickEditor(editors []EditorCandidate) (string, error) {
	var items []list.Item
	for _, e := range editors {
		if e.Installed {
			items = append(items, editorItem{c: e})
		}
	}
	if len(items) == 0 {
		return "", errors.New("no supported editor on PATH (looked for zed, cursor, code)")
	}
	if len(items) == 1 {
		return items[0].(editorItem).c.Binary, nil
	}
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
		return "", runErr
	}
	pm, ok := final.(*pickEditorModel)
	if !ok || pm.cancel {
		return "", nil
	}
	return pm.chosen, nil
}

// PickWorktree opens a Bubble Tea picker listing every worktree that has a
// directory on disk. Returns the chosen name, or "" with no error if the
// user pressed esc.
func PickWorktree(layout liftoff.Layout, prompt string) (string, error) {
	wts, err := layout.ListWorktrees()
	if err != nil {
		return "", err
	}
	st, _ := liftoff.LoadState()
	if st == nil {
		st = &liftoff.State{Worktrees: map[string]liftoff.WorktreeMeta{}}
	}
	type entry struct {
		item playWtItem
	}
	var rows []entry
	for _, w := range wts {
		if w.IsMaster(layout) || w.Bare {
			continue
		}
		name := w.Name()
		meta := st.Worktrees[name]
		ports := liftoff.PortsForSlot(meta.Slot)
		running := 0
		for _, svc := range liftoff.AllServices {
			if liftoff.StatusOf(name, svc, ports).Alive {
				running++
			}
		}
		rows = append(rows, entry{item: playWtItem{
			name:     name,
			path:     w.Path,
			emoji:    liftoff.EmojiFor(name),
			slot:     meta.Slot,
			lastUsed: meta.LastUsed,
			running:  running,
		}})
	}
	if len(rows) == 0 {
		return "", errors.New("no worktrees found — run `kit design` first")
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].item.lastUsed.After(rows[j].item.lastUsed)
	})
	items := make([]list.Item, 0, len(rows))
	for _, r := range rows {
		items = append(items, r.item)
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
