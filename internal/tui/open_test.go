package tui

import (
	"testing"

	"github.com/andreicstoica/kit/internal/liftoff"
)

func TestExecuteOpenSkipSentinelOpensNothing(t *testing.T) {
	opened, err := ExecuteOpen(OpenRequest{Name: "x", Path: "/tmp/x"},
		liftoff.SkipCandidate())
	if err != nil {
		t.Fatalf("skip should not error: %v", err)
	}
	if opened {
		t.Fatal("skip sentinel must report nothing opened")
	}
}

func TestExecuteOpenEditorTarget(t *testing.T) {
	// We can't launch a real editor in unit tests; verify dispatch reaches
	// the editor branch by using an unknown binary that will fail at launch.
	_, err := ExecuteOpen(OpenRequest{Name: "x", Path: "/tmp/x"},
		liftoff.EditorCandidate{Name: "nope", Binary: "definitely-not-an-editor-xyz", Installed: true})
	if err == nil {
		t.Fatal("expected launch failure for fake editor")
	}
}

func TestOpenRequestValidateRejectsMultipleExplicitTargets(t *testing.T) {
	err := OpenRequest{EditorFlag: "zed", Herdr: true}.Validate()
	if err == nil {
		t.Fatal("expected conflict error")
	}
}

func TestHerdrConnectForFocus(t *testing.T) {
	cases := []struct {
		ghostty, noAttach bool
		want              HerdrConnect
	}{
		{false, false, HerdrConnectAttach},
		{true, false, HerdrConnectGhostty},
		{false, true, HerdrConnectNone},
		{true, true, HerdrConnectGhostty},
	}
	for _, tc := range cases {
		if got := herdrConnectForFocus(tc.ghostty, tc.noAttach); got != tc.want {
			t.Fatalf("ghostty=%v noAttach=%v: got %v want %v", tc.ghostty, tc.noAttach, got, tc.want)
		}
	}
}

func TestGtabFromFlag(t *testing.T) {
	if gtabFromFlag(false) != liftoff.GtabSimple {
		t.Error("false should map to GtabSimple")
	}
	if gtabFromFlag(true) != liftoff.GtabDetailed {
		t.Error("true should map to GtabDetailed")
	}
}
