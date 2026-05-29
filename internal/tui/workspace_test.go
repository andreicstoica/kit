package tui

import (
	"testing"

	"github.com/andreicstoica/kit/internal/liftoff"
)

func TestOpenCandidateSkipSentinelOpensNothing(t *testing.T) {
	opened, err := openCandidate(OpenRequest{Name: "x", Path: "/tmp/x"},
		liftoff.EditorCandidate{Binary: liftoff.SkipSentinel})
	if err != nil {
		t.Fatalf("skip should not error: %v", err)
	}
	if opened {
		t.Fatal("skip sentinel must report nothing opened")
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
