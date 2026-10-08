package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/andreicstoica/kit/internal/liftoff"
)

// TestNewPlayModelOnlyCeleryCarriesBeat guards the celery/beat pairing:
// `kit play --only celery` must start beat too, or the worker runs
// without its scheduler.
func TestNewPlayModelOnlyCeleryCarriesBeat(t *testing.T) {
	t.Setenv("KIT_STATE_DIR", t.TempDir()) // isolate from real config
	layout := liftoff.Layout{Root: t.TempDir(), Master: t.TempDir()}

	// "master" resolves without an on-disk worktree, so the model builds
	// cleanly in a temp layout.
	m, err := NewPlayModel(layout, PlayConfig{Name: "master", Only: []liftoff.Service{liftoff.SvcCelery}})
	if err != nil {
		t.Fatal(err)
	}
	pm, ok := m.(*playModel)
	if !ok {
		t.Fatalf("NewPlayModel returned %T, want *playModel", m)
	}
	for _, svc := range liftoff.AllServices {
		want := svc == liftoff.SvcCelery || svc == liftoff.SvcBeat
		if pm.toggleOn[svc] != want {
			t.Errorf("toggleOn[%s] = %v, want %v", svc, pm.toggleOn[svc], want)
		}
	}
}

// A shared-broker worker holds celery and beat back. After the run, the model
// offers to replace it; "no" ends the run and "yes" reruns only the workers.
func TestPlayOffersWorkerReplaceAfterSharedBrokerSkip(t *testing.T) {
	t.Setenv("KIT_STATE_DIR", t.TempDir())
	layout := liftoff.Layout{Root: t.TempDir(), Master: t.TempDir()}
	built, err := NewPlayModel(layout, PlayConfig{Name: "master"})
	if err != nil {
		t.Fatal(err)
	}
	m := built.(*playModel)
	m.stage = playStageRun
	m.plan.Services = []liftoff.Service{liftoff.SvcApp, liftoff.SvcCelery, liftoff.SvcBeat}
	m.runOrder = m.plan.Services

	m.updateRun(playUpdMsg{ok: true, upd: liftoff.PlayUpdate{Status: liftoff.StepSkipped, SharedOwner: "other", SharedPID: 42}})
	m.updateRun(playUpdMsg{ok: false})
	if m.stage != playStageCeleryPrompt || !m.afterRun || m.celeryVictim != "other" || m.celeryPID != 42 {
		t.Fatalf("no replace prompt after run: stage=%v afterRun=%v victim=%q", m.stage, m.afterRun, m.celeryVictim)
	}

	declined := *m
	declined.updateCelery(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if declined.stage != playStageDone {
		t.Fatalf("declining should finish, stage=%v", declined.stage)
	}

	m.updateCelery(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if !m.plan.ReplaceCelery || m.plan.ReplaceVictim != "other" {
		t.Fatalf("plan = %+v", m.plan)
	}
	if len(m.plan.Services) != 2 || len(m.runOrder) != 3 {
		t.Fatalf("rerun services=%v, listed=%v; want only workers rerun, all listed", m.plan.Services, m.runOrder)
	}
}
