package tui

import "github.com/charmbracelet/lipgloss"

// Adaptive palette tuned for both Ayu Light (#f8f9fa bg) and
// Gruvbox Dark Hard (#1d2021 bg). Each color picks a darker / saturated
// variant for light backgrounds and a softer pastel for dark.
//
// lipgloss.AdaptiveColor checks the terminal background at startup and
// picks the right value automatically.
// Exported colors so cmd/ packages can reuse them (root logger, help renderer).
// Keep these tied to lipgloss.AdaptiveColor so they pick light/dark variants.
var (
	// Brand greenish — soccer pitch.
	ColorAccent = lipgloss.AdaptiveColor{Light: "#0F8A4E", Dark: "#5DD39E"}
	// Secondary text (branch names, paths).
	ColorMuted = lipgloss.AdaptiveColor{Light: "#3F4750", Dark: "#9aa5b1"}
	// Error / failure.
	ColorErr = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F38BA8"}
	// Warning / caution.
	ColorWarn = lipgloss.AdaptiveColor{Light: "#9A6700", Dark: "#F9E2AF"}
	// Success / running.
	ColorOK = lipgloss.AdaptiveColor{Light: "#1F7F3F", Dark: "#A6E3A1"}
	// Deemphasized / placeholder.
	ColorDim = lipgloss.AdaptiveColor{Light: "#7C8590", Dark: "#6c7086"}
	// Service-specific accents for log tags.
	ColorAPI     = lipgloss.AdaptiveColor{Light: "#0060c0", Dark: "#89DCEB"}
	ColorAdminBE = lipgloss.AdaptiveColor{Light: "#8b4fc6", Dark: "#cba6f7"}
	// Liftoff brand purple — deep on light bg, pastel on dark bg.
	ColorLiftoff = lipgloss.AdaptiveColor{Light: "#5B27C7", Dark: "#F6D9FA"}

	// Internal aliases (lowercase) for legacy package-internal references.
	colorAccent  = ColorAccent
	colorMuted   = ColorMuted
	colorErr     = ColorErr
	colorWarn    = ColorWarn
	colorOK      = ColorOK
	colorDim     = ColorDim
	colorAPI     = ColorAPI
	colorAdminBE = ColorAdminBE

	StyleTitle   = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	StyleHelp    = lipgloss.NewStyle().Foreground(ColorMuted)
	StyleOK      = lipgloss.NewStyle().Foreground(ColorOK)
	StyleErr     = lipgloss.NewStyle().Foreground(ColorErr)
	StyleWarn    = lipgloss.NewStyle().Foreground(ColorWarn)
	StyleDim     = lipgloss.NewStyle().Foreground(ColorDim)
	StyleHi      = lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	StyleLiftoff = lipgloss.NewStyle().Foreground(ColorLiftoff).Bold(true)
	// StyleCode renders an inline command/path/snippet. Accent color +
	// italic, no bold — distinct from regular text but lighter than StyleHi.
	StyleCode = lipgloss.NewStyle().Foreground(ColorAccent).Italic(true)
)

// Code wraps a snippet in the inline-code style. Returns "" for empty input.
func Code(s string) string {
	if s == "" {
		return ""
	}
	return StyleCode.Render(s)
}

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
