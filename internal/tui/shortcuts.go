package tui

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

const (
	// Short lists get single-character labels — 1-9 and then 0 for the tenth.
	singleDigitShortcuts = 10
	// Longer lists group a1-a9, b1-b9, … so two keys stay enough.
	shortcutsPerGroup = 9
)

// shortcutLabel returns the quick-pick label for the index-th item out of
// total. A list of at most ten items uses plain digits; longer lists add a
// letter group so every item keeps a short, unambiguous label.
func shortcutLabel(index, total int) string {
	if index <= 0 {
		return ""
	}
	if total <= singleDigitShortcuts {
		if index == singleDigitShortcuts {
			return "0"
		}
		return strconv.Itoa(index)
	}
	group := (index-1)/shortcutsPerGroup + 1
	letters := ""
	for group > 0 {
		group--
		letters = string(rune('a'+group%26)) + letters
		group /= 26
	}
	return letters + strconv.Itoa((index-1)%shortcutsPerGroup+1)
}

func shortcutTitle(title string, index, total int) string {
	if label := shortcutLabel(index, total); label != "" {
		return StyleHi.Render(label+" ") + title
	}
	return title
}

type shortcutItem interface {
	list.Item
	shortcut() string
}

type pickerShortcuts struct{ prefix string }

func shortcutHelp(l *list.Model, prefix string) {
	label := "a1…"
	if len(l.Items()) <= singleDigitShortcuts {
		label = "1…"
	}
	if prefix != "" {
		label = prefix + "_"
	}
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{key.NewBinding(key.WithKeys(label), key.WithHelp(label, "pick"))}
	}
	l.AdditionalFullHelpKeys = func() []key.Binding {
		hint := "pick by label (1-9 also work)"
		if len(l.Items()) <= singleDigitShortcuts {
			hint = "pick by label (1-9 and 0)"
		}
		return []key.Binding{key.NewBinding(key.WithKeys(label), key.WithHelp(label, hint))}
	}
}

// Labels stay attached to their items when filtering changes the visible order.
func (s *pickerShortcuts) update(k tea.KeyPressMsg, l *list.Model) (list.Item, bool) {
	defer func() { shortcutHelp(l, s.prefix) }()
	if l.FilterState() == list.Filtering {
		s.prefix = ""
		return nil, false
	}
	input := strings.ToLower(k.String())
	if s.prefix != "" && (input == "esc" || input == "backspace") {
		if input == "esc" {
			s.prefix = ""
		} else {
			s.prefix = s.prefix[:len(s.prefix)-1]
		}
		return nil, true
	}
	items := l.VisibleItems()
	if len(input) == 1 && ((input[0] >= '1' && input[0] <= '9') || input[0] == '0') {
		prefix := s.prefix
		s.prefix = ""
		if prefix == "" {
			// Short lists label items with the digit itself.
			for _, item := range items {
				if labeled, ok := item.(shortcutItem); ok && labeled.shortcut() == input {
					return item, true
				}
			}
			if input[0] >= '1' && input[0] <= '9' {
				index := int(input[0] - '1')
				if index < len(items) {
					return items[index], true
				}
			}
		} else {
			for _, item := range items {
				if labeled, ok := item.(shortcutItem); ok && labeled.shortcut() == prefix+input {
					return item, true
				}
			}
		}
		return nil, true
	}
	if len(input) == 1 && input[0] >= 'a' && input[0] <= 'z' {
		for _, prefix := range []string{s.prefix + input, input} {
			for _, item := range items {
				if labeled, ok := item.(shortcutItem); ok && strings.HasPrefix(labeled.shortcut(), prefix) {
					s.prefix = prefix
					return nil, true
				}
			}
		}
	}
	s.prefix = ""
	return nil, false
}
