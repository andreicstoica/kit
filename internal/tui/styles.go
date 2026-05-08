package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Brand greenish — matches a soccer pitch.
	colorAccent = lipgloss.Color("#5DD39E")
	colorMuted  = lipgloss.Color("#6c7086")
	colorErr    = lipgloss.Color("#F38BA8")
	colorWarn   = lipgloss.Color("#F9E2AF")
	colorOK     = lipgloss.Color("#A6E3A1")
	colorDim    = lipgloss.Color("#7f849c")

	StyleTitle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	StyleHelp  = lipgloss.NewStyle().Foreground(colorMuted)
	StyleOK    = lipgloss.NewStyle().Foreground(colorOK)
	StyleErr   = lipgloss.NewStyle().Foreground(colorErr)
	StyleWarn  = lipgloss.NewStyle().Foreground(colorWarn)
	StyleDim   = lipgloss.NewStyle().Foreground(colorDim)
	StyleHi    = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
)

// Glyph returns a unicode marker for a step status.
func Glyph(status string) string {
	switch status {
	case "pending":
		return "○"
	case "running":
		return "●"
	case "done":
		return "✓"
	case "skipped":
		return "·"
	case "failed":
		return "✗"
	}
	return "?"
}
