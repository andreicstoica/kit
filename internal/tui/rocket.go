package tui

import (
	"fmt"
	"math/rand"

	tea "github.com/charmbracelet/bubbletea"
)

// rocket is a launch: 3·2·1 countdown, then the rocket climbs off-screen
// trailing exhaust while a starfield scrolls down. Launch tally below.
type rocket struct {
	c   canvas
	rng *rand.Rand

	y     float64 // rocket nose row
	stars []star

	phase    penaltyPhase
	phaseT   int
	frame    int
	launches int
}

type star struct {
	x, y, spd float64
}

const (
	rkCountdown = 30 // frames of 3·2·1·GO
	rkFlight    = 30 // frames of climb
)

func newRocket(rng *rand.Rand) Animation {
	r := &rocket{c: newCanvas(animWidth, animHeight), rng: rng}
	r.stars = make([]star, 16)
	for i := range r.stars {
		r.stars[i] = star{
			x:   float64(rng.Intn(animWidth)),
			y:   float64(rng.Intn(animHeight)),
			spd: 0.15 + rng.Float64()*0.35,
		}
	}
	r.padY()
	return r
}

func (r *rocket) padY() { r.y = float64(r.c.h) - 3 }

func (r *rocket) Init() tea.Cmd { return animTick() }

func (r *rocket) Update(msg tea.Msg) (Animation, tea.Cmd) {
	if _, ok := msg.(animTickMsg); !ok {
		return r, nil
	}
	r.phaseT++
	r.frame++
	r.scrollStars()
	switch r.phase {
	case phasePreShoot:
		if r.phaseT > rkCountdown {
			r.launches++
			r.phase = phaseShoot
			r.phaseT = 0
		}
	case phaseShoot:
		r.y -= float64(r.c.h+4) / rkFlight // climb fully off the top
		if r.phaseT > rkFlight {
			r.phase = phaseResult
			r.phaseT = 0
		}
	case phaseResult:
		if r.phaseT > 18 {
			r.padY()
			r.phase = phasePreShoot
			r.phaseT = 0
		}
	}
	return r, animTick()
}

// scrollStars drifts the starfield downward, faster during the climb.
func (r *rocket) scrollStars() {
	boost := 1.0
	if r.phase == phaseShoot {
		boost = 3.0
	}
	for i := range r.stars {
		r.stars[i].y += r.stars[i].spd * boost
		if r.stars[i].y >= float64(r.c.h) {
			r.stars[i].y = 0
			r.stars[i].x = float64(r.rng.Intn(r.c.w))
		}
	}
}

func (r *rocket) View() string {
	c := &r.c
	c.clear()

	for _, s := range r.stars {
		c.setf(s.x, s.y, '·', stDim)
	}

	// Launch pad gantry at the bottom.
	c.hline(c.w/2-3, c.w/2+3, c.h-2, '═', stFrame)

	r.drawRocket()

	switch {
	case r.phase == phasePreShoot:
		c.textCenter(c.h/2, r.countdownText(), stWarn)
	case r.phase == phaseResult:
		c.textCenter(c.h/2, "LIFTOFF!", stOK)
	}

	if r.launches > 0 {
		c.textCenter(c.h-1, fmt.Sprintf("launches %d", r.launches), stDim)
	}
	return c.render()
}

func (r *rocket) countdownText() string {
	n := 3 - r.phaseT/10
	if n <= 0 {
		return "GO!"
	}
	return fmt.Sprintf("%d", n)
}

// drawRocket renders the rocket body plus flickering exhaust during the climb.
func (r *rocket) drawRocket() {
	cx := r.c.w / 2
	ny := int(r.y)
	r.c.set(cx, ny, '▲', stBall)
	r.c.set(cx, ny+1, '█', stActor)
	r.c.set(cx-1, ny+2, '◣', stFrame)
	r.c.set(cx, ny+2, '█', stActor)
	r.c.set(cx+1, ny+2, '◢', stFrame)

	if r.phase == phaseShoot {
		flames := []rune{'░', '▒', '▓'}
		for d := 1; d <= 3; d++ {
			g := flames[(r.frame+d)%len(flames)]
			r.c.set(cx, ny+2+d, g, stWarn)
		}
	}
}
