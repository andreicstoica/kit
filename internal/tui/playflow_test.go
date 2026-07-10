package tui

import (
	"testing"

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
