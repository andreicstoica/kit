package tui

import (
	"fmt"
	"strings"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/charmbracelet/lipgloss"
)

// RenderLineup prints a static (non-interactive) table of active worktrees.
// Used by `kit lineup` / `kit ls`.
func RenderLineup(layout liftoff.Layout) (string, error) {
	wts, err := layout.ListWorktrees()
	if err != nil {
		return "", err
	}
	type row struct {
		name, branch, status, db, gtab string
		isLegacy                       bool
	}
	var rows []row
	for _, w := range wts {
		if w.IsMaster(layout) || w.Bare {
			continue
		}
		name := w.Name()
		st := "clean"
		if liftoff.IsDirty(w.Path) {
			st = "dirty"
		}
		ahead, behind := layout.AheadBehind(w.Path)
		if ahead > 0 || behind > 0 {
			st = fmt.Sprintf("%s ↑%d↓%d", st, ahead, behind)
		}
		db := "—"
		if liftoff.HasPostgres() && liftoff.HasDB(name) {
			db = liftoff.DBName(name)
		}
		gtab := "—"
		if layout.HasGtab(name) {
			gtab = "✓"
		}
		rows = append(rows, row{name: name, branch: w.Branch, status: st, db: db, gtab: gtab, isLegacy: w.HasLegacyPrefix()})
	}

	header := lipgloss.NewStyle().Bold(true).Foreground(colorAccent)

	var b strings.Builder
	b.WriteString(header.Render(fmt.Sprintf("%-30s %-30s %-18s %-22s %s", "NAME", "BRANCH", "STATUS", "DB", "GTAB")) + "\n")

	if len(rows) == 0 {
		b.WriteString(StyleDim.Render("no kits on the field. start one with `kit dress`.") + "\n")
		return b.String(), nil
	}

	for _, r := range rows {
		nameDisp := r.name
		if r.isLegacy {
			nameDisp = r.name + StyleDim.Render(" (legacy)")
		}
		stStyle := StyleOK
		if strings.Contains(r.status, "dirty") {
			stStyle = StyleWarn
		}
		b.WriteString(fmt.Sprintf("%-30s %-30s %-18s %-22s %s\n",
			nameDisp,
			r.branch,
			stStyle.Render(fmt.Sprintf("%-18s", r.status)),
			r.db,
			r.gtab,
		))
	}
	b.WriteString("\n")
	b.WriteString(StyleDim.Render(fmt.Sprintf("master: %s", layout.Master)) + "\n")
	return b.String(), nil
}
