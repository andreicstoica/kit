package tui

import (
	"math/rand"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// TestAnimationsStableSize drives every animation through many frames (covering
// every phase) and asserts the rendered box never changes dimensions — the
// no-layout-shift invariant — and never panics.
func TestAnimationsStableSize(t *testing.T) {
	for i, mk := range animConstructors {
		rng := rand.New(rand.NewSource(int64(i) + 1))
		a := mk(rng)
		first := a.View()
		wantW, wantH := lipgloss.Width(first), lipgloss.Height(first)
		for f := 0; f < 300; f++ {
			a, _ = a.Update(animTickMsg(time.Time{}))
			v := a.View()
			if w, h := lipgloss.Width(v), lipgloss.Height(v); w != wantW || h != wantH {
				t.Errorf("%T frame %d: size %dx%d != initial %dx%d", a, f, w, h, wantW, wantH)
				break
			}
		}
	}
}

// TestAnimationFrames logs one frame of each animation deep enough to have
// reached its result phase, for visual inspection via `go test -run
// TestAnimationFrames -v`.
func TestAnimationFrames(t *testing.T) {
	for i, mk := range animConstructors {
		rng := rand.New(rand.NewSource(int64(i) + 1))
		a := mk(rng)
		for f := 0; f < 70; f++ {
			a, _ = a.Update(animTickMsg(time.Time{}))
		}
		t.Logf("\n%T:\n%s\n", a, a.View())
	}
}
