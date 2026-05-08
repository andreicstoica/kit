package tui

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
)

// KeyMap is the shared keybinding registry for kit TUIs.
type KeyMap struct {
	Up      key.Binding
	Down    key.Binding
	Toggle  key.Binding
	Enter   key.Binding
	Back    key.Binding
	Filter  key.Binding
	Yes     key.Binding
	No      key.Binding
	Cancel  key.Binding
	HelpKey key.Binding
	Quit    key.Binding
}

// ShortHelp returns the keys that always appear in the footer.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Enter, k.Back, k.Quit, k.HelpKey}
}

// FullHelp returns the keys shown when help is expanded.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Enter, k.Back},
		{k.Toggle, k.Filter, k.Yes, k.No},
		{k.Cancel, k.HelpKey, k.Quit},
	}
}

// DefaultKeymap is the keymap used by every flow.
var DefaultKeymap = KeyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Toggle: key.NewBinding(
		key.WithKeys(" ", "tab"),
		key.WithHelp("space", "toggle"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select / continue"),
	),
	Back: key.NewBinding(
		key.WithKeys("backspace"),
		key.WithHelp("backspace", "back"),
	),
	Filter: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "filter"),
	),
	Yes: key.NewBinding(
		key.WithKeys("y", "Y"),
		key.WithHelp("y", "yes"),
	),
	No: key.NewBinding(
		key.WithKeys("n", "N"),
		key.WithHelp("n", "no"),
	),
	Cancel: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "cancel"),
	),
	HelpKey: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c", "q"),
		key.WithHelp("q/ctrl+c", "quit"),
	),
}

// NewHelp returns a configured help.Model for kit TUIs.
func NewHelp() help.Model {
	h := help.New()
	h.ShowAll = false
	h.ShortSeparator = " · "
	h.FullSeparator = "   "
	h.Styles.ShortKey = h.Styles.ShortKey.Foreground(colorAccent).Bold(true)
	h.Styles.ShortDesc = h.Styles.ShortDesc.Foreground(colorMuted)
	h.Styles.FullKey = h.Styles.FullKey.Foreground(colorAccent).Bold(true)
	h.Styles.FullDesc = h.Styles.FullDesc.Foreground(colorMuted)
	h.Styles.Ellipsis = h.Styles.Ellipsis.Foreground(colorDim)
	return h
}
