package tui

import (
	"fmt"
	"math"
	"math/rand"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/harmonica"
)

// penaltyPhase tracks where the penalty-kick animation is in its loop.
type penaltyPhase int

const (
	phasePreShoot penaltyPhase = iota // ball + keeper at rest, brief pause
	phaseShoot                        // ball flying, keeper diving
	phaseResult                       // SAVE!/GOAL! flash + net ripple
)

// soccer is a penalty kick: ball springs to a corner, keeper dives, verdict
// flashes. Ball spins + trails in flight, crowd jumps up top, net ripples on a
// goal.
type soccer struct {
	c   canvas
	rng *rand.Rand

	ballXSpring harmonica.Spring
	ballYSpring harmonica.Spring
	keepSpring  harmonica.Spring

	ballX, ballY   float64
	ballVX, ballVY float64
	keepX          float64
	keepVX         float64

	targetBallX, targetBallY float64
	targetKeepX              float64
	restBallX, restBallY     float64
	restKeepX                float64

	// Recent ball positions, newest last, for the flight trail.
	trailX, trailY []float64

	phase  penaltyPhase
	phaseT int
	frame  int // monotonic frame counter, drives crowd shimmer
	saved  bool
	shots  int
	saves  int
}

// Goal sits down a couple rows so the top two are free for the jumping crowd.
const (
	soGoalTop    = 2
	soGoalBottom = 6
)

func newSoccer(rng *rand.Rand) Animation {
	c := newCanvas(animWidth, animHeight)
	o := &soccer{
		c:           c,
		rng:         rng,
		ballXSpring: harmonica.NewSpring(harmonica.FPS(30), 6.0, 0.55),
		ballYSpring: harmonica.NewSpring(harmonica.FPS(30), 6.0, 0.55),
		keepSpring:  harmonica.NewSpring(harmonica.FPS(30), 5.0, 0.45),
		restBallX:   float64(animWidth) / 2,
		restBallY:   float64(animHeight) - 3,
		restKeepX:   float64(animWidth) / 2,
	}
	o.ballX, o.ballY = o.restBallX, o.restBallY
	o.keepX = o.restKeepX
	o.targetBallX, o.targetBallY = o.restBallX, o.restBallY
	o.targetKeepX = o.restKeepX
	return o
}

func (o *soccer) Init() tea.Cmd { return animTick() }

func (o *soccer) Update(msg tea.Msg) (Animation, tea.Cmd) {
	if _, ok := msg.(animTickMsg); !ok {
		return o, nil
	}
	o.phaseT++
	o.frame++
	switch o.phase {
	case phasePreShoot:
		if o.phaseT > 18 { // ~600ms at rest, then aim
			o.aim()
			o.trailX, o.trailY = o.trailX[:0], o.trailY[:0]
			o.phase = phaseShoot
			o.phaseT = 0
		}
	case phaseShoot:
		o.ballX, o.ballVX = o.ballXSpring.Update(o.ballX, o.ballVX, o.targetBallX)
		o.ballY, o.ballVY = o.ballYSpring.Update(o.ballY, o.ballVY, o.targetBallY)
		o.keepX, o.keepVX = o.keepSpring.Update(o.keepX, o.keepVX, o.targetKeepX)
		o.pushTrail(o.ballX, o.ballY)
		if o.phaseT > 30 { // ~1s of flight
			o.judge()
			o.phase = phaseResult
			o.phaseT = 0
		}
	case phaseResult:
		if o.phaseT > 24 {
			o.ballX, o.ballY = o.restBallX, o.restBallY
			o.keepX = o.restKeepX
			o.ballVX, o.ballVY, o.keepVX = 0, 0, 0
			o.phase = phasePreShoot
			o.phaseT = 0
		}
	}
	return o, animTick()
}

// pushTrail records a ball position, keeping the most recent few.
func (o *soccer) pushTrail(x, y float64) {
	const maxTrail = 4
	o.trailX = append(o.trailX, x)
	o.trailY = append(o.trailY, y)
	if len(o.trailX) > maxTrail {
		o.trailX = o.trailX[1:]
		o.trailY = o.trailY[1:]
	}
}

// aim picks a target corner for the ball and a guess for the keeper.
func (o *soccer) aim() {
	corners := []float64{4.5, float64(o.c.w) - 5.5}
	heights := []float64{3.0, 5.0}
	o.targetBallX = corners[o.rng.Intn(2)]
	o.targetBallY = heights[o.rng.Intn(2)]
	if o.rng.Float64() < 0.45 { // 45% chance to guess the right side
		o.targetKeepX = o.targetBallX
	} else if o.targetBallX < float64(o.c.w)/2 {
		o.targetKeepX = corners[1]
	} else {
		o.targetKeepX = corners[0]
	}
}

// judge decides save vs goal from keeper/ball proximity at the end of flight.
func (o *soccer) judge() {
	o.saved = math.Abs(o.keepX-o.ballX) < 2.5 && o.ballY < float64(soGoalBottom)
	o.shots++
	if o.saved {
		o.saves++
	}
}

func (o *soccer) View() string {
	c := &o.c
	c.clear()
	w := c.w
	goalLeft, goalRight := 3, w-4

	o.drawCrowd()

	// Goal frame.
	c.hline(goalLeft, goalRight, soGoalTop, '─', stFrame)
	c.set(goalLeft, soGoalTop, '┌', stFrame)
	c.set(goalRight, soGoalTop, '┐', stFrame)
	c.vline(goalLeft, soGoalTop+1, soGoalBottom, '│', stFrame)
	c.vline(goalRight, soGoalTop+1, soGoalBottom, '│', stFrame)

	// Net cross-hatch.
	for row := soGoalTop + 1; row <= soGoalBottom-1; row++ {
		for col := goalLeft + 1; col <= goalRight-1; col++ {
			if (row+col)%3 == 0 {
				c.set(col, row, '·', stDim)
			}
		}
	}

	// Penalty spot.
	c.set(w/2, c.h-2, '×', stDim)

	o.drawKeeper(goalLeft, goalRight)
	o.drawTrail()
	o.drawBall()
	o.drawResult()
	o.drawScore()

	return c.render()
}

// drawCrowd draws fans hopping up and down in a wave, arms up at the peak.
func (o *soccer) drawCrowd() {
	for fx := 3; fx < o.c.w-2; fx += 4 {
		up := math.Sin(float64(o.frame)*0.25+float64(fx)) > 0
		row, st := 1, stDim
		if up { // peak: arms up
			row, st = 0, stActor
		}
		o.c.set(fx, row, 'o', st)
		if up {
			o.c.set(fx-1, row, '\\', st)
			o.c.set(fx+1, row, '/', st)
		}
	}
}

func (o *soccer) drawKeeper(goalLeft, goalRight int) {
	row := soGoalBottom - 1
	col := clampInt(int(math.Round(o.keepX)), goalLeft+1, goalRight-1)
	diving := math.Abs(o.keepX-o.restKeepX) > 1.5
	armChar := '─'
	if diving {
		armChar = '═'
	}
	if o.phase == phaseShoot {
		if col-1 > goalLeft {
			o.c.set(col-1, row, armChar, stActor)
		}
		if col+1 < goalRight {
			o.c.set(col+1, row, armChar, stActor)
		}
	}
	o.c.set(col, row, '◇', stActor)
}

// drawTrail draws fading dots behind the ball during flight.
func (o *soccer) drawTrail() {
	if o.phase != phaseShoot {
		return
	}
	glyphs := []rune{'∙', '•', '◦'}
	for i := range o.trailX {
		g := glyphs[i*len(glyphs)/maxInt(len(o.trailX), 1)]
		o.c.setf(o.trailX[i], o.trailY[i], g, stDim)
	}
}

func (o *soccer) drawBall() {
	ball := '●'
	if o.phase == phaseShoot {
		// Spin through quadrant glyphs while flying.
		spin := []rune{'◐', '◓', '◑', '◒'}
		ball = spin[o.frame%len(spin)]
	}
	o.c.setf(o.ballX, o.ballY, ball, stBall)
}

// drawResult flashes the verdict and ripples the net on a goal.
func (o *soccer) drawResult() {
	if o.phase != phaseResult {
		return
	}
	if o.saved {
		o.c.textCenter(o.c.h/2, "SAVE!", stActor)
		return
	}
	o.c.textCenter(o.c.h/2, "GOOAL!", stOK)
	// Net ripple: scatter sparkles near where the ball hit, pulsing by frame.
	ripple := []rune{'✦', '✧', '*'}
	bx, by := int(math.Round(o.ballX)), int(math.Round(o.ballY))
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			n := o.phaseT + dx + dy
			if n%2 == 0 {
				o.c.set(bx+dx, by+dy, ripple[(n%len(ripple)+len(ripple))%len(ripple)], stOK)
			}
		}
	}
}

func (o *soccer) drawScore() {
	if o.shots == 0 {
		return
	}
	o.c.textCenter(o.c.h-1, fmt.Sprintf("saves %d · goals %d", o.saves, o.shots-o.saves), stDim)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
