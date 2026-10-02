package tui

import (
	"strings"
	"testing"

	"github.com/andreicstoica/kit/internal/liftoff"
)

func TestMergedWash_ReportsFailedCandidate(t *testing.T) {
	m := &mergedModel{
		stage: mergedStageRun,
		candidates: []liftoff.MergedCandidate{{
			Name: "lost", Path: t.TempDir(), Branch: "lost", Head: "old-head",
		}},
		selected: map[int]bool{0: true},
	}
	msg := m.startRun()()
	m.Update(msg)
	if !m.failed || m.failureErr == nil || m.stage != mergedStageDone {
		t.Fatalf("failed cleanup reported as success: %+v", m)
	}
	if got := m.viewDone(); !strings.Contains(got, "failed") || strings.Contains(got, "complete") {
		t.Fatalf("failed cleanup view = %q", got)
	}
}
