package tui

import (
	"strings"
	"testing"
)

// These tests render the first frame of the shared forms without a TTY, by
// building the form and calling Init()+View() directly. They guard the
// first-frame visibility fix: huh's select viewport starts at height 0, so
// without an explicit Height the options + "> " cursor are blank until a key
// lands. selectHeight()/buildSelectForm must keep them visible from frame 1.

func TestRunSelectFirstFrameShowsAllOptions(t *testing.T) {
	opts := []SelectOption[string]{
		{Label: "Simple (2 tabs)", Value: "s"},
		{Label: "Detailed (5 tabs)", Value: "d"},
		{Label: "Skip — don't open", Value: ""},
	}
	val := "s"
	f := buildSelectForm("Ghostty workspace layout", "pick a layout", opts, &val)
	_ = f.Init()
	view := f.View()

	for _, o := range opts {
		if !strings.Contains(view, o.Label) {
			t.Errorf("first frame missing option %q\n%s", o.Label, view)
		}
	}
	if !strings.Contains(view, "> ") {
		t.Errorf("first frame missing selection cursor\n%s", view)
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

func TestSelectHeightReservesRows(t *testing.T) {
	// title + wrapped description + every option + slack.
	if got := selectHeight(3); got != 7 {
		t.Errorf("selectHeight(3) = %d, want 7", got)
	}
}
