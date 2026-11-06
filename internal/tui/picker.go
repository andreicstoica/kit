package tui

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

// listPicker handles standalone worktree, editor, and adopt selection.
type listPicker struct {
	list      list.Model
	shortcuts pickerShortcuts
	chosen    list.Item
	cancel    bool
}

func (m *listPicker) Init() tea.Cmd { return nil }

func (m *listPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height-2)
	case tea.BackgroundColorMsg:
		ApplyTheme(msg.IsDark(), nil, &m.list)
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.cancel = true
			return m, tea.Quit
		}
		if item, handled := m.shortcuts.update(msg, &m.list); handled {
			if item != nil {
				m.chosen = item
				return m, tea.Quit
			}
			return m, nil
		}
		if m.list.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancel = true
			return m, tea.Quit
		case "enter":
			if it := m.list.SelectedItem(); it != nil {
				m.chosen = it
				return m, tea.Quit
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *listPicker) View() tea.View { return NewAltView(m.list.View()) }

// ListPickerConfig configures a single-select list picker.
type ListPickerConfig struct {
	Title  string
	Items  []list.Item
	Filter bool // enable "/" fuzzy filtering
}

// RunListPicker presents the shared picker over pre-built items and returns
// the chosen item. ok is false when the user cancelled (esc/ctrl+c). Callers
// type-assert the returned item back to their concrete item type to read its
// payload. Items are expected to already implement list.DefaultItem
// (Title/Description/FilterValue); use NewListDelegate-rendered items.
func RunListPicker(cfg ListPickerConfig) (chosen list.Item, ok bool, err error) {
	l := list.New(cfg.Items, NewListDelegate(), 0, 0)
	StyleList(&l, cfg.Title, cfg.Filter)
	m := &listPicker{list: l}
	final, runErr := tea.NewProgram(m).Run()
	if runErr != nil {
		return nil, false, runErr
	}
	pm, isModel := final.(*listPicker)
	if !isModel || pm.cancel || pm.chosen == nil {
		return nil, false, nil
	}
	return pm.chosen, true, nil
}
