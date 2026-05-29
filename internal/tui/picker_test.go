package tui

import (
	"testing"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// These tests drive the shared listPicker's Update loop directly with key
// messages — no TTY/program needed. This is the standard way to unit-test a
// Bubble Tea model: feed it messages, inspect the resulting state.

func newTestPicker(filter bool, names ...string) *listPicker {
	items := make([]list.Item, 0, len(names))
	for i, n := range names {
		items = append(items, editorItem{
			c:          liftoff.EditorCandidate{Name: n, Binary: n, Installed: true},
			displayIdx: i + 1,
		})
	}
	l := list.New(items, NewListDelegate(), 40, 20)
	StyleList(&l, "test picker", filter)
	m := &listPicker{list: l}
	// Seed a size like the program would.
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	return m
}

func mkKey(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestPickerEnterSelectsHighlighted(t *testing.T) {
	m := newTestPicker(false, "Zed", "Cursor", "VS Code")
	m.Update(mkKey("enter"))
	if m.cancel {
		t.Fatal("enter should not cancel")
	}
	got, ok := m.chosen.(editorItem)
	if !ok || got.c.Name != "Zed" {
		t.Fatalf("enter chose %+v, want Zed", m.chosen)
	}
}

func TestPickerNumericQuickPick(t *testing.T) {
	m := newTestPicker(false, "Zed", "Cursor", "VS Code")
	m.Update(mkKey("2"))
	got, ok := m.chosen.(editorItem)
	if !ok || got.c.Name != "Cursor" {
		t.Fatalf("quick-pick 2 chose %+v, want Cursor", m.chosen)
	}
}

func TestPickerQuickPickOutOfRangeIgnored(t *testing.T) {
	m := newTestPicker(false, "Zed", "Cursor")
	m.Update(mkKey("9"))
	if m.chosen != nil {
		t.Fatalf("out-of-range quick-pick should not select, got %+v", m.chosen)
	}
}

func TestPickerEscCancels(t *testing.T) {
	m := newTestPicker(false, "Zed", "Cursor")
	m.Update(mkKey("esc"))
	if !m.cancel {
		t.Fatal("esc should cancel")
	}
	if m.chosen != nil {
		t.Fatalf("cancel should leave chosen nil, got %+v", m.chosen)
	}
}

func TestPickerDigitsGoToFilterWhileFiltering(t *testing.T) {
	m := newTestPicker(true, "Zed", "Cursor", "VS Code")
	// Enter filtering mode, then type a digit — it must NOT quick-pick.
	m.Update(mkKey("/"))
	if m.list.FilterState() != list.Filtering {
		t.Skip("list did not enter filtering mode in this bubbles version")
	}
	m.Update(mkKey("2"))
	if m.chosen != nil {
		t.Fatalf("digit while filtering should not quick-pick, got %+v", m.chosen)
	}
}
