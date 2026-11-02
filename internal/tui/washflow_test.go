package tui

import (
	tea "charm.land/bubbletea/v2"
	"os/exec"
	"strings"
	"testing"

	"github.com/andreicstoica/kit/internal/liftoff"
)

func TestWashDistinguishesUnmergedFromUnpublishedCommits(t *testing.T) {
	m := washModel{layout: liftoff.Layout{MainBranch: "master"}, selected: washItem{name: "feature", aheadCount: 12, publicationKnown: true, unpublishedCount: 0}}
	view := m.viewConfirm()
	for _, text := range []string{"12 commit(s) are not on master", "0 commit(s) absent from cached remote refs", "Remote branches are not deleted"} {
		if !strings.Contains(view, text) {
			t.Fatalf("missing %q in %s", text, view)
		}
	}
	if strings.Contains(view, "will be permanently deleted") {
		t.Fatal("unmerged commits were treated as necessarily unpublished")
	}
}

func TestWashNamedMissingWorkspaceDoesNotOfferAnother(t *testing.T) {
	root := t.TempDir()
	if out, err := exec.Command("git", "init", root).CombinedOutput(); err != nil {
		t.Fatalf("init: %s: %v", out, err)
	}
	_, err := NewWashModelFor(liftoff.Layout{Root: root, Master: root}, "gone")
	if err == nil || !strings.Contains(err.Error(), "kit reconcile") {
		t.Fatalf("missing target error = %v", err)
	}
}

func TestWashConfirmationTracksCurrentSelection(t *testing.T) {
	m := &washModel{dropDB: true, removeGtab: true}
	m.pickWash(washItem{name: "first"})
	m.pickWash(washItem{name: "second", hasDB: true, hasGtab: true, dirty: true})
	if !m.dropDB || !m.removeGtab || m.confirmCursor != 0 {
		t.Fatal("cleanup choices leaked between workspaces")
	}
	m.updateConfirm(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.confirmArmed {
		t.Fatal("destructive confirmation was not armed")
	}
	m.updateConfirm(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if m.confirmArmed {
		t.Fatal("changing the plan retained the previous confirmation")
	}
}
