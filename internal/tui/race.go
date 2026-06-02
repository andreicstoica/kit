package tui

import (
	"fmt"
	"math"
	"math/rand"

	tea "github.com/charmbracelet/bubbletea"
)

// race is a two-lane sprint to a checkered finish. Runners have random speeds
// plus jitter, legs cycle, dust trails behind. Near-tie = PHOTO!, else WIN!.
type race struct {
	c   canvas
	rng *rand.Rand

	x [2]float64 // runner positions
	v [2]float64 // base speeds

	phase  penaltyPhase
	phaseT int
	frame  int
	winner int // lane that won the finished race
	photo  bool
	wins   [2]int
	races  int
}

const (
	raceStartX = 2
	raceFinish = animWidth - 5
)

func newRace(rng *rand.Rand) Animation {
	r := &race{c: newCanvas(animWidth, animHeight), rng: rng}
	r.reset()
	return r
}

func (r *race) reset() {
	r.x = [2]float64{raceStartX, raceStartX}
	r.v = [2]float64{0.7 + r.rng.Float64()*0.5, 0.7 + r.rng.Float64()*0.5}
}

func (r *race) lanes() (int, int) { return r.c.h/2 - 1, r.c.h/2 + 1 }

func (r *race) Init() tea.Cmd { return animTick() }

func (r *race) Update(msg tea.Msg) (Animation, tea.Cmd) {
	if _, ok := msg.(animTickMsg); !ok {
		return r, nil
	}
	r.phaseT++
	r.frame++
	switch r.phase {
	case phasePreShoot:
		if r.phaseT > 16 { // on your marks…
			r.phase = phaseShoot
			r.phaseT = 0
		}
	case phaseShoot:
		for i := range r.x {
			r.x[i] += r.v[i] * (0.6 + r.rng.Float64()*0.8) // surge + jitter
		}
		if r.x[0] >= raceFinish || r.x[1] >= raceFinish {
			r.finish()
			r.phase = phaseResult
			r.phaseT = 0
		}
	case phaseResult:
		if r.phaseT > 22 {
			r.reset()
			r.phase = phasePreShoot
			r.phaseT = 0
		}
	}
	return r, animTick()
}

func (r *race) finish() {
	r.races++
	if r.x[0] >= r.x[1] {
		r.winner = 0
	} else {
		r.winner = 1
	}
	r.photo = math.Abs(r.x[0]-r.x[1]) < 1.5
	r.wins[r.winner]++
}

func (r *race) View() string {
	c := &r.c
	c.clear()
	lane0, lane1 := r.lanes()

	// Track lines + checkered finish.
	c.hline(raceStartX, raceFinish, lane0, '┄', stFrame)
	c.hline(raceStartX, raceFinish, lane1, '┄', stFrame)
	for _, row := range []int{lane0 - 1, lane0, lane1, lane1 + 1} {
		r.checker(raceFinish+1, row)
	}

	r.drawRunner(0, lane0, stActor)
	r.drawRunner(1, lane1, stBall)

	if r.phase == phaseResult {
		if r.photo {
			c.textCenter(2, "PHOTO!", stWarn)
		} else {
			c.textCenter(2, "WIN!", stOK)
		}
	}
	if r.races > 0 {
		c.textCenter(c.h-1, fmt.Sprintf("you %d · cpu %d", r.wins[0], r.wins[1]), stDim)
	}
	return c.render()
}

func (r *race) checker(x, row int) {
	if row%2 == 0 {
		r.c.set(x, row, '▓', stFrame)
	} else {
		r.c.set(x, row, '░', stFrame)
	}
}

// drawRunner draws one sprinter with a two-frame leg cycle and a dust trail.
func (r *race) drawRunner(i, row, style int) {
	glyphs := []rune{'ƙ', 'ʀ'}
	g := glyphs[(r.frame/3)%len(glyphs)]
	if r.phase != phaseShoot {
		g = 'ʀ'
	}
	x := int(math.Round(r.x[i]))
	// Dust streaming behind while running.
	if r.phase == phaseShoot {
		for d := 1; d <= 3; d++ {
			if (r.frame+d)%2 == 0 {
				r.c.set(x-d, row, '·', stDim)
			}
		}
	}
	r.c.set(x, row, g, style)
}
