package tui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/andreicstoica/kit/internal/liftoff"
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

func mkKey(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEsc}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	default:
		r := []rune(s)[0]
		return tea.KeyPressMsg{Code: r, Text: s}
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

func TestPickerTwoKeyShortcutBeyondNine(t *testing.T) {
	names := make([]string, 15)
	for i := range names {
		names[i] = fmt.Sprintf("workspace-%02d", i+1)
	}
	m := newTestPicker(false, names...)
	m.Update(mkKey("b"))
	if m.chosen != nil {
		t.Fatalf("prefix selected an item: %+v", m.chosen)
	}
	m.Update(mkKey("3"))
	got, ok := m.chosen.(editorItem)
	if !ok || got.c.Name != "workspace-12" {
		t.Fatalf("b3 chose %+v, want workspace-12", m.chosen)
	}
}

func TestPickerShortcutAfterFilterUsesDisplayedLabel(t *testing.T) {
	m := newTestPicker(true, "Zed", "Cursor", "VS Code")
	m.list.SetFilterText("Cursor")
	if got := m.list.VisibleItems()[0].(editorItem).Title(); !strings.Contains(got, "a2") {
		t.Fatalf("filtered row title = %q, want a2 shortcut", got)
	}
	m.Update(mkKey("a"))
	m.Update(mkKey("2"))
	got, ok := m.chosen.(editorItem)
	if !ok || got.c.Name != "Cursor" {
		t.Fatalf("a2 chose %+v, want Cursor", m.chosen)
	}
}

func TestPickerUnfinishedShortcut(t *testing.T) {
	names := make([]string, 15)
	for i := range names {
		names[i] = fmt.Sprintf("workspace-%02d", i+1)
	}
	for _, tc := range []struct {
		name string
		keys []string
		want string
	}{
		{"invalid suffix", []string{"b", "9"}, ""},
		{"escape clears prefix", []string{"b", "esc", "3"}, "workspace-03"},
		{"backspace clears prefix", []string{"b", "backspace", "3"}, "workspace-03"},
		{"navigation clears prefix", []string{"b", "down", "3"}, "workspace-03"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestPicker(false, names...)
			for _, k := range tc.keys {
				m.Update(mkKey(k))
			}
			got := ""
			if item, ok := m.chosen.(editorItem); ok {
				got = item.c.Name
			}
			if got != tc.want || m.cancel {
				t.Fatalf("keys %v chose %q, cancel=%v; want %q", tc.keys, got, m.cancel, tc.want)
			}
		})
	}
}

func TestPickerShortcutTextBelongsToFilter(t *testing.T) {
	m := newTestPicker(true, "a1", "b3")
	m.Update(mkKey("/"))
	m.Update(mkKey("a"))
	m.Update(mkKey("1"))
	if m.chosen != nil || m.list.FilterValue() != "a1" {
		t.Fatalf("filter=%q chosen=%+v", m.list.FilterValue(), m.chosen)
	}
}
