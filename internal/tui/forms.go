package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// Shared huh form helpers. Every short-menu select and yes/no confirm in kit
// goes through these so they share one theme, one set of defaults, and the
// first-frame sizing fix — instead of each call site re-specifying them (and
// drifting). See the design-language note in ui.go.

// SelectOption is one entry in a RunSelect menu.
type SelectOption[T comparable] struct {
	Label string
	Value T
}

// RunSelect presents a short single-choice menu and returns the chosen value.
// def is the initially-highlighted value.
//
// Height is set explicitly so the options and the "> " cursor render on the
// very first frame. Without it huh's viewport height starts at 0 and the list
// stays blank until a key/WindowSize msg lands — a race new users lose.
func RunSelect[T comparable](title, description string, opts []SelectOption[T], def T) (T, error) {
	val := def
	err := buildSelectForm(title, description, opts, &val).Run()
	return val, err
}

// buildSelectForm constructs (but does not run) the select form. Split out so
// tests can render the first frame without a TTY.
func buildSelectForm[T comparable](title, description string, opts []SelectOption[T], val *T) *huh.Form {
	hopts := make([]huh.Option[T], 0, len(opts))
	for _, o := range opts {
		hopts = append(hopts, huh.NewOption(o.Label, o.Value))
	}
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[T]().
				Title(title).
				Description(description).
				Options(hopts...).
				Height(selectHeight(len(opts))).
				Value(val),
		),
	).WithTheme(KitHuhTheme()).
		WithShowHelp(true).
		WithShowErrors(true)
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

// selectHeight reserves rows for the title + (wrapped) description + every
// option, so the whole menu is visible on the first frame.
func selectHeight(numOptions int) int { return numOptions + 4 }

func isConfirmYes(k tea.KeyMsg) bool {
	return k.Type == tea.KeyEnter || strings.EqualFold(k.String(), "y")
}

func isConfirmNo(k tea.KeyMsg) bool {
	return strings.EqualFold(k.String(), "n")
}

func confirmHelp(yesLabel, noLabel string) string {
	return StyleHelp.Render("enter/y: " + yesLabel + " · n: " + noLabel + " · esc: cancel")
}
