package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// These tests render the first frame of the shared forms without a TTY, by
// building the model/form and calling Init()+View() directly. They guard the
// first-frame contract: every option, its 1-9 quick-pick number, and the "> "
// cursor must be visible from frame 1 — no key/WindowSize msg required.

func TestRunSelectFirstFrameShowsAllOptions(t *testing.T) {
	opts := []SelectOption[string]{
		{Label: "Simple (2 tabs)", Value: "s"},
		{Label: "Detailed (5 tabs)", Value: "d"},
		{Label: "Skip — don't open", Value: ""},
	}
	m := newSelectModel("Ghostty workspace layout", "pick a layout", opts, "s")
	_ = m.Init()
	view := m.View()

	for _, o := range opts {
		if !strings.Contains(view, o.Label) {
			t.Errorf("first frame missing option %q\n%s", o.Label, view)
		}
	}
	for _, prefix := range []string{"1 ", "2 "} {
		if !strings.Contains(view, prefix) {
			t.Errorf("first frame missing quick-pick number %q\n%s", prefix, view)
		}
	}
	if !strings.Contains(view, "> ") {
		t.Errorf("first frame missing selection cursor\n%s", view)
	}
}

func TestRunSelectDigitQuickPick(t *testing.T) {
	opts := []SelectOption[string]{
		{Label: "First", Value: "a"},
		{Label: "Second", Value: "b"},
		{Label: "Third", Value: "c"},
	}
	m := newSelectModel("pick one", "", opts, "a")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if cmd == nil {
		t.Fatal("expected quit cmd after digit quick-pick")
	}
	if m.cancel {
		t.Fatal("digit quick-pick must not cancel")
	}
	if m.chosen != 1 {
		t.Fatalf("chosen = %d, want 1", m.chosen)
	}
	if got := m.opts[m.chosen].Value; got != "b" {
		t.Fatalf("chosen value = %q, want %q", got, "b")
	}
}

func TestRunConfirmFirstFrameShowsButtons(t *testing.T) {
	val := true
	f := buildConfirmForm(ConfirmConfig{Title: "Delete contents?", Affirmative: "Yes, clear", Negative: "Cancel"}, &val)
	_ = f.Init()
	view := f.View()

	for _, want := range []string{"Delete contents?", "Yes, clear", "Cancel"} {
		if !strings.Contains(view, want) {
			t.Errorf("confirm first frame missing %q\n%s", want, view)
		}
	}
}

func TestRunConfirmDefaultsToYesNo(t *testing.T) {
	val := true
	f := buildConfirmForm(ConfirmConfig{Title: "Proceed?"}, &val)
	_ = f.Init()
	view := f.View()
	if !strings.Contains(view, "Yes") || !strings.Contains(view, "No") {
		t.Errorf("expected default Yes/No buttons\n%s", view)
	}
}
