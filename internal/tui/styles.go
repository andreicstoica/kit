package tui

import (
	"image/color"

	lipgloss "charm.land/lipgloss/v2"
)

// Adaptive palette tuned for both Ayu Light (#f8f9fa bg) and
// Gruvbox Dark Hard (#1d2021 bg). Each color picks a darker / saturated
// variant for light backgrounds and a softer pastel for dark.
//
// Lip Gloss v2 removed AdaptiveColor, so the theme is explicit: default
// dark (kit developers live in dark terminals), updated asynchronously
// from tea.BackgroundColorMsg via SetDarkBackground. No blocking terminal
// query happens at startup.
var (
	darkBackground = true

	// Brand greenish — soccer pitch.
	ColorAccent color.Color
	// Secondary text (branch names, paths).
	ColorMuted color.Color
	// Error / failure.
	ColorErr color.Color
	// Warning / caution.
	ColorWarn color.Color
	// Success / running.
	ColorOK color.Color
	// Deemphasized / placeholder.
	ColorDim color.Color
	// Service-specific accents for log tags.
	ColorAPI     color.Color
	ColorAdminBE color.Color
	// Liftoff brand purple — deep on light bg, pastel on dark bg.
	ColorLiftoff color.Color

	// Internal aliases (lowercase) for legacy package-internal references.
	colorAccent  color.Color
	colorMuted   color.Color
	colorErr     color.Color
	colorWarn    color.Color
	colorOK      color.Color
	colorDim     color.Color
	colorAPI     color.Color
	colorAdminBE color.Color

	StyleTitle   lipgloss.Style
	StyleHelp    lipgloss.Style
	StyleOK      lipgloss.Style
	StyleErr     lipgloss.Style
	StyleWarn    lipgloss.Style
	StyleDim     lipgloss.Style
	StyleHi      lipgloss.Style
	StyleLiftoff lipgloss.Style
	// StyleCode renders an inline command/path/snippet. Accent color +
	// italic, no bold — distinct from regular text but lighter than StyleHi.
	StyleCode lipgloss.Style
)

// IsDark reports the current theme assumption.
func IsDark() bool { return darkBackground }

// AccentFor resolves the brand accent for an explicit theme. Widgets that
// resolve their own dark/light state per render (huh themes) use this
// instead of the package-level ColorAccent.
func AccentFor(isDark bool) color.Color {
	return lipgloss.LightDark(isDark)(lipgloss.Color("#0F8A4E"), lipgloss.Color("#5DD39E"))
}

// SetDarkBackground switches the palette and rebuilds the shared styles.
// Flows call this from Update on tea.BackgroundColorMsg.
func SetDarkBackground(dark bool) {
	darkBackground = dark
	buildStyles()
}

func buildStyles() {
	ld := lipgloss.LightDark(darkBackground)
	ColorAccent = ld(lipgloss.Color("#0F8A4E"), lipgloss.Color("#5DD39E"))
	ColorMuted = ld(lipgloss.Color("#3F4750"), lipgloss.Color("#9aa5b1"))
	ColorErr = ld(lipgloss.Color("#B91C1C"), lipgloss.Color("#F38BA8"))
	ColorWarn = ld(lipgloss.Color("#9A6700"), lipgloss.Color("#F9E2AF"))
	ColorOK = ld(lipgloss.Color("#1F7F3F"), lipgloss.Color("#A6E3A1"))
	ColorDim = ld(lipgloss.Color("#7C8590"), lipgloss.Color("#6c7086"))
	ColorAPI = ld(lipgloss.Color("#0060c0"), lipgloss.Color("#89DCEB"))
	ColorAdminBE = ld(lipgloss.Color("#8b4fc6"), lipgloss.Color("#cba6f7"))
	ColorLiftoff = ld(lipgloss.Color("#5B27C7"), lipgloss.Color("#F6D9FA"))

	colorAccent = ColorAccent
	colorMuted = ColorMuted
	colorErr = ColorErr
	colorWarn = ColorWarn
	colorOK = ColorOK
	colorDim = ColorDim
	colorAPI = ColorAPI
	colorAdminBE = ColorAdminBE

	StyleTitle = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	StyleHelp = lipgloss.NewStyle().Foreground(ColorMuted)
	StyleOK = lipgloss.NewStyle().Foreground(ColorOK)
	StyleErr = lipgloss.NewStyle().Foreground(ColorErr)
	StyleWarn = lipgloss.NewStyle().Foreground(ColorWarn)
	StyleDim = lipgloss.NewStyle().Foreground(ColorDim)
	StyleHi = lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	StyleLiftoff = lipgloss.NewStyle().Foreground(ColorLiftoff).Bold(true)
	StyleCode = lipgloss.NewStyle().Foreground(ColorAccent).Italic(true)

	// TitleStyle aliases StyleTitle — keep it pointed at the rebuilt value.
	TitleStyle = StyleTitle
}

func init() { buildStyles() }

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
