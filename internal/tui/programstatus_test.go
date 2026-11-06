package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/andreicstoica/kit/internal/liftoff"
)

func TestPlayViewReportsProgramStatus(t *testing.T) {
	layout := liftoff.Layout{Root: t.TempDir(), Master: t.TempDir()}
	m, err := NewPlayModel(layout, PlayConfig{Name: "master"})
	if err != nil {
		t.Fatal(err)
	}
	view := m.(interface{ View() tea.View }).View()
	if view.ProgramStatus == nil {
		t.Fatal("want ProgramStatus, got nil")
	}
	if view.ProgramStatus.App != "kit" {
		t.Errorf("app = %q, want kit", view.ProgramStatus.App)
	}
	if view.ProgramStatus.State != tea.ProgramStateBlocked {
		t.Errorf("state = %q, want blocked at picker", view.ProgramStatus.State)
	}
	if view.MouseMode != tea.MouseModeCellMotion {
		t.Errorf("mouse = %v, want CellMotion", view.MouseMode)
	}
}
