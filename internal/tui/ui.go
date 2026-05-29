package tui

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
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
//      (workspace layout, root menu). Built with huh via RunSelect. Always
//      sized so the options + "> " cursor render on the first frame.
//
//   3. Confirm      — a yes/no decision. Built with huh via RunConfirm.
//      Always affirmative-left / negative-right with consistent wording.
//
// Color tokens live in styles.go; this file holds the shared component
// builders that bind those tokens to the Charm widgets. Reach for these
// helpers rather than constructing list/huh widgets inline so a styling
// change here propagates across the whole CLI.

// TitleStyle is the bold-accent heading used atop every picker and form.
var TitleStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)

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

// KitHuhTheme is the shared huh theme for kit's selects and confirms. It
// starts from ThemeCharm (so adaptive light/dark colors are honored) and
// retints the focused accents to kit's brand green for a consistent look
// with the list pickers.
func KitHuhTheme() *huh.Theme {
	t := huh.ThemeCharm()
	t.Focused.Title = t.Focused.Title.Foreground(ColorAccent).Bold(true)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(ColorAccent)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(ColorAccent)
	t.Focused.FocusedButton = t.Focused.FocusedButton.Background(ColorAccent)
	return t
}
