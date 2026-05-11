package tui

import (
	"fmt"
	"strings"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/charmbracelet/lipgloss"
)

// RenderDoctor renders a static report of doctor check results.
func RenderDoctor(results []liftoff.CheckResult) string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("kit doctor — checking your setup…") + "\n\n")

	nameW := 12
	for _, r := range results {
		if r.Status == liftoff.CheckSkip {
			continue
		}
		glyph, style := doctorGlyphStyle(r.Status)
		name := padRight(r.Name, nameW)
		b.WriteString(style.Render(glyph) + " " + style.Render(name) + "  " + r.Detail + "\n")
		if r.FixHint != "" && r.Status != liftoff.CheckOK {
			b.WriteString(strings.Repeat(" ", 4+nameW) + StyleDim.Render("fix: "+r.FixHint) + "\n")
		}
	}

	s := liftoff.Summarize(results)
	b.WriteString("\n")
	b.WriteString(StyleDim.Render(fmt.Sprintf("%d ok · %d warning · %d failure", s.OK, s.Warn, s.Fail)) + "\n")
	return b.String()
}

func doctorGlyphStyle(s liftoff.CheckStatus) (string, lipgloss.Style) {
	switch s {
	case liftoff.CheckOK:
		return "✓", StyleOK
	case liftoff.CheckWarn:
		return "⚠", StyleWarn
	case liftoff.CheckFail:
		return "✗", StyleErr
	}
	return "·", StyleDim
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}
