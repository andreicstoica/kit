package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/andreicstoica/kit/internal/liftoff"
)

func TestPlayModelRendersAtFixedWindowSize(t *testing.T) {
	layout := liftoff.Layout{Root: t.TempDir(), Master: t.TempDir()}
	m, err := NewPlayModel(layout, PlayConfig{Name: "master"})
	if err != nil {
		t.Skip("needs master repo")
	}
	p := tea.NewProgram(m, tea.WithWindowSize(80, 24), tea.WithInput(nil), tea.WithOutput(nil))
	if p == nil {
		t.Fatal("nil program")
	}
	// The model should handle a WindowSizeMsg at 80x24 without panic and
	// still render content.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := updated.(tea.Model).(interface{ View() tea.View }).View()
	if view.Content == "" {
		t.Fatal("empty view at 80x24")
	}
}
