package tui

import (
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
