package tui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// kit UI design language
// =======================
//
// Every interactive prompt in kit is built from one of three primitives so
// the experience is consistent no matter which command you're in:
//
//   1. List picker  — choosing one thing from a set that may be long or
//      filterable (a worktree, an editor, an adopt candidate). Built with
//      bubbles/list via RunListPicker. Always: accent-highlighted selection,
//      1-9 numeric quick-pick, esc/ctrl+c cancels, "/" filters when enabled.
//
//   2. Select       — choosing one option from a short, fixed menu
//      (workspace layout, root menu). A hand-rolled inline bubbletea model
//      via RunSelect. Always: "> " cursor, 1-9 numeric quick-pick, options
//      visible on the first frame, esc/ctrl+c cancels.
//
//   3. Confirm      — a yes/no decision. Built with huh via RunConfirm.
//      Always affirmative-left / negative-right with consistent wording.
//
// Color tokens live in styles.go; this file holds the shared component
// builders that bind those tokens to the Charm widgets. Reach for these
// helpers rather than constructing list/huh widgets inline so a styling
// change here propagates across the whole CLI.

// TitleStyle is the bold-accent heading used atop every picker and form.
var TitleStyle = StyleTitle

// NewListDelegate returns the shared bubbles/list delegate: a default
// two-line delegate with kit's accent color on the selected row. Every
// list-based picker (RunListPicker, play, pause) uses this so selection
// highlighting looks identical everywhere.
func NewListDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.
		Foreground(ColorAccent).BorderForeground(ColorAccent)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.
		Foreground(ColorAccent).BorderForeground(ColorAccent)
	return d
}

// StyleList applies kit's shared title style and help/filter defaults to a
// freshly-built list.Model. Centralizes the per-list boilerplate that was
// previously copy-pasted into every picker.
func StyleList(l *list.Model, title string, filtering bool) {
	l.Title = title
	l.Styles.Title = TitleStyle
	l.SetShowHelp(true)
	l.SetFilteringEnabled(filtering)
}

// NewAltView wraps content in a fullscreen alternate-screen view. Flows
// whose program previously passed tea.WithAltScreen set this on every
// View return instead (Bubble Tea v2 moved screen mode into the view).
func NewAltView(content string) tea.View {
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// RestyleList reapplies kit's list chrome for the current theme.
func RestyleList(l *list.Model) {
	l.Styles.Title = TitleStyle
	l.SetDelegate(NewListDelegate())
}

// ApplyTheme switches the global theme from a background-color message and
// restyles the given help model and lists. Flow Updates call this on
// tea.BackgroundColorMsg so light terminals restyle without a restart.
func ApplyTheme(isDark bool, h *help.Model, lists ...*list.Model) {
	SetDarkBackground(isDark)
	if h != nil {
		*h = RestyleHelp(*h)
	}
	for _, l := range lists {
		if l != nil {
			RestyleList(l)
		}
	}
}

// KitHuhTheme is the shared huh theme for kit's selects and confirms. It
// starts from ThemeCharm (so adaptive light/dark colors are honored) and
// retints the focused accents to kit's brand green for a consistent look
// with the list pickers.
func KitHuhTheme() huh.Theme {
	return huh.ThemeFunc(func(isDark bool) *huh.Styles {
		t := huh.ThemeCharm(isDark)
		accent := AccentFor(isDark)
		t.Focused.Title = t.Focused.Title.Foreground(accent).Bold(true)
		t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(accent)
		t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(accent)
		t.Focused.FocusedButton = t.Focused.FocusedButton.Background(accent)
		return t
	})
}
