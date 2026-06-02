package tui

import (
	"fmt"
	"math"
	"math/rand"

	tea "github.com/charmbracelet/bubbletea"
)

// factory is an assembly line: a package rides the conveyor, gets stamped by
// presses, ships off the right edge. Shipped tally below.
type factory struct {
	c   canvas
	rng *rand.Rand

	pkgX     float64
	stations []int // belt columns where presses sit

	phase   penaltyPhase
	phaseT  int
	frame   int
	shipped int
}

const (
	fcBeltRow = animHeight - 3
	fcStartX  = 2
)

func newFactory(rng *rand.Rand) Animation {
	f := &factory{c: newCanvas(animWidth, animHeight), rng: rng}
	w := f.c.w
	f.stations = []int{w / 4, w / 2, 3 * w / 4}
	f.pkgX = fcStartX
	f.phase = phaseShoot
	return f
}

func (f *factory) Init() tea.Cmd { return animTick() }

func (f *factory) Update(msg tea.Msg) (Animation, tea.Cmd) {
	if _, ok := msg.(animTickMsg); !ok {
		return f, nil
	}
	f.phaseT++
	f.frame++
	switch f.phase {
	case phaseShoot:
		f.pkgX += 0.6
		if f.pkgX >= float64(f.c.w-3) {
			f.shipped++
			f.phase = phaseResult
			f.phaseT = 0
		}
	case phaseResult:
		if f.phaseT > 16 {
			f.pkgX = fcStartX
			f.phase = phaseShoot
			f.phaseT = 0
		}
	}
	return f, animTick()
}

// stamping reports whether a press at column x is currently pressed onto the
// package passing beneath it.
func (f *factory) stamping(x int) bool {
	return f.phase == phaseShoot && math.Abs(f.pkgX-float64(x)) < 1.0
}

func (f *factory) View() string {
	c := &f.c
	c.clear()

	f.drawBelt()
	f.drawStations()

	// Package on the belt (hidden once it has shipped off the edge).
	if f.phase == phaseShoot {
		c.setf(f.pkgX, fcBeltRow-1, '▣', stBall)
	}

	if f.phase == phaseResult {
		c.textCenter(c.h/2, "SHIPPED!", stOK)
	}
	c.textCenter(c.h-1, fmt.Sprintf("shipped %d", f.shipped), stDim)
	return c.render()
}

// drawBelt draws the conveyor surface and a scrolling tread underneath.
func (f *factory) drawBelt() {
	c := &f.c
	c.hline(1, c.w-2, fcBeltRow, '═', stFrame)
	for x := 1; x < c.w-1; x++ {
		if (x+f.frame)%3 == 0 {
			c.set(x, fcBeltRow+1, '╱', stDim)
		}
	}
}

// drawStations draws each press, lowered with a spark while stamping.
func (f *factory) drawStations() {
	for _, x := range f.stations {
		f.c.vline(x, 1, fcBeltRow-3, '│', stFrame)
		if f.stamping(x) {
			f.c.set(x, fcBeltRow-2, '▼', stWarn)
			f.c.set(x, fcBeltRow-1, '✦', stWarn)
		} else {
			f.c.set(x, fcBeltRow-2, '┳', stFrame)
		}
	}
}
