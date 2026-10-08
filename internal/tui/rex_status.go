package tui

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/andreicstoica/kit/internal/liftoff"
)

func RenderRexProgramStatus(state liftoff.RexState) string {
	t := table.New().Headers("WORKSPACE", "TAB", "PROGRAM", "STATE", "DETAIL").StyleFunc(func(row, col int) lipgloss.Style {
		return lipgloss.NewStyle().Padding(0, 1).Bold(row == table.HeaderRow)
	})
	for _, session := range state.Sessions {
		for _, window := range session.Windows {
			for _, block := range window.Blocks {
				if len(block.ProgramStatus.Records) == 0 {
					t.Row(session.Label, window.Label, "—", "unknown", "no OSC 7501 report")
				}
				for _, record := range block.ProgramStatus.Records {
					app := record.EffectiveApp
					if app == "" {
						app = record.App
					}
					if app == "" {
						app = "—"
					}
					status := record.State
					if record.Kind != "" {
						status += " (" + record.Kind + ")"
					}
					t.Row(session.Label, window.Label, app, status, record.Message)
				}
			}
		}
	}
	return t.String() + "\n"
}
