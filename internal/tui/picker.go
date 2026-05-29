package tui

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// listPicker is the single, shared single-select list model behind every
// standalone picker in kit (worktree, editor, adopt candidate). It replaces
// three near-identical models that each re-implemented the same keybindings,
// quick-pick, and styling.
//
// Keybindings (consistent everywhere):
//   - enter        select the highlighted item
//   - 1-9          quick-pick the Nth visible (filter-aware) item
//   - esc / ctrl+c cancel
//   - "/"          filter (when the list has filtering enabled)
type listPicker struct {
	list   list.Model
	chosen list.Item
	cancel bool
}

func (m *listPicker) Init() tea.Cmd { return nil }

func (m *listPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height-2)
	case tea.KeyMsg:
		// While filtering, keystrokes belong to the filter input — don't
		// hijack digits as quick-pick or esc as cancel-the-program.
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
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			// Numeric quick-pick: jump to the Nth item in the current
			// (possibly filtered) view.
			idx := int(msg.String()[0] - '0' - 1)
			items := m.list.VisibleItems()
			if idx >= 0 && idx < len(items) {
				m.chosen = items[idx]
				return m, tea.Quit
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *listPicker) View() string { return m.list.View() }

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
	final, runErr := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if runErr != nil {
		return nil, false, runErr
	}
	pm, isModel := final.(*listPicker)
	if !isModel || pm.cancel || pm.chosen == nil {
		return nil, false, nil
	}
	return pm.chosen, true, nil
}
