package tui

import (
	"fmt"
	"math"
	"math/rand"

	tea "github.com/charmbracelet/bubbletea"
)

// basketball is a jump shot: ball arcs to the top-right hoop, then swishes or
// clanks off the rim and bounces away. Made/attempted shows below.
type basketball struct {
	c   canvas
	rng *rand.Rand

	x0, y0 float64 // launch point
	make   bool    // does this attempt go in?
	defl   float64 // horizontal deflection sign on a miss

	phase  penaltyPhase
	phaseT int
	frame  int
	made   int
	shots  int
}

// Hoop geometry (top-right of the canvas).
const (
	bbFlight  = 26 // frames of arc
	bbRimRow  = 3
	bbArcPeak = 6.0 // how far above the launch/target line the arc bows
)

func newBasketball(rng *rand.Rand) Animation {
	b := &basketball{c: newCanvas(animWidth, animHeight), rng: rng}
	b.reset()
	return b
}

func (b *basketball) reset() {
	b.x0 = 5 + b.rng.Float64()*4
	b.y0 = float64(b.c.h) - 3
	b.make = b.rng.Float64() < 0.6
	if b.rng.Float64() < 0.5 {
		b.defl = -1
	} else {
		b.defl = 1
	}
}

func (b *basketball) rimCenter() (float64, float64) {
	return float64(b.c.w) - 7, float64(bbRimRow + 1)
}

func (b *basketball) Init() tea.Cmd { return animTick() }

func (b *basketball) Update(msg tea.Msg) (Animation, tea.Cmd) {
	if _, ok := msg.(animTickMsg); !ok {
		return b, nil
	}
	b.phaseT++
	b.frame++
	switch b.phase {
	case phasePreShoot:
		if b.phaseT > 14 {
			b.phase = phaseShoot
			b.phaseT = 0
		}
	case phaseShoot:
		if b.phaseT > bbFlight {
			b.shots++
			if b.make {
				b.made++
			}
			b.phase = phaseResult
			b.phaseT = 0
		}
	case phaseResult:
		if b.phaseT > 22 {
			b.reset()
			b.phase = phasePreShoot
			b.phaseT = 0
		}
	}
	return b, animTick()
}

// ballPos returns the ball position for the current frame.
func (b *basketball) ballPos() (float64, float64) {
	rx, ry := b.rimCenter()
	switch b.phase {
	case phaseShoot:
		t := float64(b.phaseT) / bbFlight
		x := b.x0 + (rx-b.x0)*t
		y := b.y0 + (ry-b.y0)*t - bbArcPeak*math.Sin(math.Pi*t)
		return x, y
	case phaseResult:
		t := float64(b.phaseT)
		if b.make {
			return rx, ry + t*0.35 // drop straight through the net
		}
		return rx + b.defl*t*0.4, ry - 0.5 + t*0.3 // clank and bounce away
	}
	return b.x0, b.y0
}

func (b *basketball) View() string {
	c := &b.c
	c.clear()
	w := c.w

	b.drawCourt()
	b.drawShooter()

	// Backboard + rim + net in the top-right.
	rimLeft, rimRight := w-9, w-5
	c.vline(w-4, bbRimRow-2, bbRimRow+1, '│', stFrame) // backboard post
	c.hline(rimLeft, rimRight, bbRimRow, '━', stWarn)  // rim
	for row := bbRimRow + 1; row <= bbRimRow+2; row++ {
		for col := rimLeft + 1; col < rimRight; col++ {
			c.set(col, row, '╫', stDim) // net
		}
	}

	// Ball.
	bx, by := b.ballPos()
	ball := '●'
	if b.phase == phaseShoot && b.frame%2 == 0 {
		ball = '◍' // subtle spin
	}
	c.setf(bx, by, ball, stBall)

	// Verdict.
	if b.phase == phaseResult {
		if b.make {
			c.textCenter(c.h/2, "SWISH!", stOK)
		} else {
			c.textCenter(c.h/2, "RIM!", stErr)
		}
	}

	if b.shots > 0 {
		c.textCenter(c.h-1, fmt.Sprintf("made %d / %d", b.made, b.shots), stDim)
	}
	return c.render()
}

// drawShooter draws the shooter: standing ready, then leaping arms-up as the
// ball releases.
func (b *basketball) drawShooter() {
	floor := b.c.h - 2
	sx := int(b.x0)
	// Hop up for the first half of the shot, then come back down.
	jumping := b.phase == phaseShoot && b.phaseT < bbFlight
	hy := floor - 2
	if jumping {
		hy-- // leave the ground
	}
	b.c.set(sx, hy, 'O', stActor)   // head
	b.c.set(sx, hy+1, '│', stActor) // torso
	if jumping {
		b.c.set(sx-1, hy, '\\', stActor) // arms up
		b.c.set(sx+1, hy, '/', stActor)
		b.c.set(sx-1, hy+2, '/', stActor) // legs tucked
		b.c.set(sx+1, hy+2, '\\', stActor)
	} else {
		b.c.set(sx-1, hy+1, '/', stActor) // arms down, ready
		b.c.set(sx+1, hy+1, '\\', stActor)
		b.c.set(sx, hy+2, 'Λ', stActor) // standing legs
	}
}

// drawCourt draws the floor line and a faint free-throw arc.
func (b *basketball) drawCourt() {
	floor := b.c.h - 2
	b.c.hline(1, b.c.w-2, floor, '─', stFrame)
	for x := 4; x < b.c.w-4; x++ {
		if (x+b.frame/4)%6 == 0 {
			b.c.set(x, floor, '┄', stDim)
		}
	}
}
