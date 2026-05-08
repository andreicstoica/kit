package tui

import "github.com/charmbracelet/lipgloss"

// Adaptive palette tuned for both Ayu Light (#f8f9fa bg) and
// Gruvbox Dark Hard (#1d2021 bg). Each color picks a darker / saturated
// variant for light backgrounds and a softer pastel for dark.
//
// lipgloss.AdaptiveColor checks the terminal background at startup and
// picks the right value automatically.
var (
	// Brand greenish — soccer pitch.
	colorAccent = lipgloss.AdaptiveColor{Light: "#0F8A4E", Dark: "#5DD39E"}
	// Secondary text (branch names, paths).
	colorMuted  = lipgloss.AdaptiveColor{Light: "#3F4750", Dark: "#9aa5b1"}
	// Error / failure.
	colorErr    = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F38BA8"}
	// Warning / caution.
	colorWarn   = lipgloss.AdaptiveColor{Light: "#9A6700", Dark: "#F9E2AF"}
	// Success / running.
	colorOK     = lipgloss.AdaptiveColor{Light: "#1F7F3F", Dark: "#A6E3A1"}
	// Deemphasized / placeholder.
	colorDim    = lipgloss.AdaptiveColor{Light: "#7C8590", Dark: "#6c7086"}

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
