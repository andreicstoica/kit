package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// Shared short-menu helpers. Every short-menu select and yes/no confirm in kit
// goes through these so they share one theme, one set of defaults, and one set
// of keybindings — instead of each call site re-specifying them (and drifting).
// See the design-language note in ui.go.

// SelectOption is one entry in a RunSelect menu.
type SelectOption[T comparable] struct {
	Label string
	Value T
}

// RunSelect presents a short single-choice menu and returns the chosen value.
// def is the initially-highlighted value.
//
// Keybindings mirror the list pickers (see picker.go): up/k and down/j move,
// 1-9 quick-picks the Nth option, enter selects, esc/ctrl+c cancels. On cancel
// it returns (def, huh.ErrUserAborted) — the same sentinel the old huh-based
// implementation surfaced, which callers already special-case.
func RunSelect[T comparable](title, description string, opts []SelectOption[T], def T) (T, error) {
	m := newSelectModel(title, description, opts, def)
	final, err := tea.NewProgram(m).Run()
	if err != nil {
		return def, err
	}
	sm, ok := final.(*selectModel[T])
	if !ok || sm.cancel || sm.chosen < 0 {
		return def, huh.ErrUserAborted
	}
	return sm.opts[sm.chosen].Value, nil
}

// selectModel is the hand-rolled bubbletea model behind RunSelect: a short,
// fixed menu with the same keybindings as listPicker (1-9 quick-pick, enter,
// esc/ctrl+c) rendered inline so the whole menu is visible on the first frame.
type selectModel[T comparable] struct {
	title       string
	description string
	opts        []SelectOption[T]
	cursor      int
	chosen      int // index of the selected option; -1 until selected
	cancel      bool
}

// newSelectModel constructs (but does not run) the select model. Split out so
// tests can render the first frame without a TTY. The cursor starts on the
// option whose Value == def (index 0 if def is not among the options).
func newSelectModel[T comparable](title, description string, opts []SelectOption[T], def T) *selectModel[T] {
	m := &selectModel[T]{
		title:       title,
		description: description,
		opts:        opts,
		chosen:      -1,
	}
	for i, o := range opts {
		if o.Value == def {
			m.cursor = i
			break
		}
	}
	return m
}

func (m *selectModel[T]) Init() tea.Cmd { return nil }

func (m *selectModel[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "ctrl+c", "esc":
			m.cancel = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.opts)-1 {
				m.cursor++
			}
		case "enter":
			m.chosen = m.cursor
			return m, tea.Quit
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			// Numeric quick-pick: jump straight to the Nth option.
			idx := int(k.String()[0] - '0' - 1)
			if idx >= 0 && idx < len(m.opts) {
				m.chosen = idx
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m *selectModel[T]) View() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render(m.title) + "\n")
	if m.description != "" {
		b.WriteString(StyleDim.Render(m.description) + "\n")
	}
	b.WriteString("\n")
	for i, o := range m.opts {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		// Same numbering convention as the list pickers: only options 1-9
		// get a quick-pick prefix; a 10th+ option shows none.
		prefix := ""
		if i+1 < 10 {
			prefix = StyleHi.Render(fmt.Sprintf("%d ", i+1))
		}
		b.WriteString(cursor + prefix + o.Label + "\n")
	}
	b.WriteString("\n" + StyleHelp.Render("↑/↓ move · 1-9 pick · enter select · esc cancel") + "\n")
	return b.String()
}

// ConfirmConfig configures a yes/no prompt. Affirmative/Negative default to
// "Yes"/"No"; override only when different wording is semantically needed
// (e.g. "Skip").
type ConfirmConfig struct {
	Title       string
	Description string
	Affirmative string
	Negative    string
	Default     bool
}

// RunConfirm presents a consistent yes/no prompt and returns the choice.
func RunConfirm(cfg ConfirmConfig) (bool, error) {
	val := cfg.Default
	err := buildConfirmForm(cfg, &val).Run()
	return val, err
}

// buildConfirmForm constructs (but does not run) the confirm form, applying
// the Yes/No defaults. Split out so tests can render it without a TTY.
func buildConfirmForm(cfg ConfirmConfig, val *bool) *huh.Form {
	aff := cfg.Affirmative
	if aff == "" {
		aff = "Yes"
	}
	neg := cfg.Negative
	if neg == "" {
		neg = "No"
	}
	c := huh.NewConfirm().
		Title(cfg.Title).
		Affirmative(aff).
		Negative(neg).
		Value(val)
	if cfg.Description != "" {
		c = c.Description(cfg.Description)
	}
	return huh.NewForm(huh.NewGroup(c)).
		WithTheme(KitHuhTheme()).
		WithShowHelp(true).
		WithShowErrors(true)
}

func isConfirmYes(k tea.KeyMsg) bool {
	return k.Type == tea.KeyEnter || strings.EqualFold(k.String(), "y")
}

func isConfirmNo(k tea.KeyMsg) bool {
	return strings.EqualFold(k.String(), "n")
}

func confirmHelp(yesLabel, noLabel string) string {
	return StyleHelp.Render("enter/y: " + yesLabel + " · n: " + noLabel + " · esc: cancel")
}
