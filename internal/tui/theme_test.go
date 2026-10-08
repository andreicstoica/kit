package tui

import (
	"image/color"
	"math"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

func TestConfirmButtonContrastInBothThemes(t *testing.T) {
	for _, dark := range []bool{false, true} {
		theme := KitHuhTheme().Theme(dark)
		for _, style := range []lipgloss.Style{theme.Focused.FocusedButton, theme.Focused.BlurredButton, theme.Blurred.FocusedButton, theme.Blurred.BlurredButton} {
			fg, bg := luminance(style.GetForeground()), luminance(style.GetBackground())
			if fg < bg {
				fg, bg = bg, fg
			}
			if ratio := (fg + 0.05) / (bg + 0.05); ratio < 4.5 {
				t.Errorf("dark=%v button contrast %.2f, need 4.5", dark, ratio)
			}
		}
	}
}

func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	linear := func(v uint32) float64 {
		x := float64(v) / 65535
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(r) + 0.7152*linear(g) + 0.0722*linear(b)
}

// Theme regression tests for the Bubble Tea v2 migration. Lip Gloss v2
// removed AdaptiveColor, so kit resolves light/dark explicitly. These guard
// that the explicit param is honored (not the stale global) and that list
// chrome is rebuilt for light terminals.

// AccentFor takes an explicit theme so widgets that resolve their own
// dark/light state (huh themes) get the right accent. It must not return
// the global theme's accent when asked for the other one.
func TestAccentForHonorsParam(t *testing.T) {
	defer SetDarkBackground(true)
	SetDarkBackground(true) // global = dark

	if got := AccentFor(false); got != lipgloss.Color("#0F8A4E") {
		t.Errorf("AccentFor(false) = %v, want light accent #0F8A4E", got)
	}
	if got := AccentFor(true); got != lipgloss.Color("#5DD39E") {
		t.Errorf("AccentFor(true) = %v, want dark accent #5DD39E", got)
	}
}

// The shared list delegate must render unselected rows for the current
// theme. bubbles v2 hardcodes dark item styles in NewDefaultDelegate, so
// kit must rebuild from NewDefaultItemStyles(IsDark()).
func TestListDelegateHonorsLightTheme(t *testing.T) {
	defer SetDarkBackground(true)
	SetDarkBackground(false)

	d := NewListDelegate()
	if got := d.Styles.NormalTitle.GetForeground(); got != lipgloss.Color("#1a1a1a") {
		t.Errorf("light NormalTitle foreground = %v, want #1a1a1a", got)
	}
}

// RestyleList must rebuild the full list chrome for the current theme,
// not just the title and selection. StatusBar and pagination dots keep
// their dark colors otherwise.
func TestRestyleListRebuildsLightChrome(t *testing.T) {
	defer SetDarkBackground(true)
	SetDarkBackground(false)

	l := list.New([]list.Item{}, NewListDelegate(), 40, 20)
	RestyleList(&l)

	if got := l.Styles.StatusBar.GetForeground(); got != lipgloss.Color("#A49FA5") {
		t.Errorf("light StatusBar foreground = %v, want #A49FA5", got)
	}
	wantDots := list.DefaultStyles(false)
	if l.Paginator.ActiveDot != wantDots.ActivePaginationDot.String() {
		t.Errorf("ActiveDot = %q, want %q", l.Paginator.ActiveDot, wantDots.ActivePaginationDot.String())
	}
}

// The mute-terminal background probe must quit on its own. Bubble Tea v2
// sends no fallback BackgroundColorMsg, so without a timeout the program
// blocks forever and static commands hang.
func TestDetectModelTimeoutQuits(t *testing.T) {
	m := &detectModel{dark: true}
	if cmd := m.Init(); cmd == nil {
		t.Fatal("detect Init must schedule the background query and a timeout")
	}
	if _, cmd := m.Update(detectTimeoutMsg{}); cmd == nil {
		t.Fatal("detect timeout must quit the probe program")
	}
	// A real answer still wins over the default.
	m2 := &detectModel{dark: true}
	light := tea.BackgroundColorMsg{Color: lipgloss.Color("#f8f9fa")}
	if _, cmd := m2.Update(light); cmd == nil {
		t.Fatal("detect background answer must quit the probe program")
	}
	if m2.dark {
		t.Fatal("light terminal answer must flip the probe to light")
	}
}
