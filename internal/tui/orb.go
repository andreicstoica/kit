package tui

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/lipgloss"
)

// orbTickMsg fires every animation frame.
type orbTickMsg time.Time

// orbTick scheduled next frame at 30 fps.
func orbTick() tea.Cmd {
	return tea.Tick(33*time.Millisecond, func(t time.Time) tea.Msg {
		return orbTickMsg(t)
	})
}

// Animation phases.
type penaltyPhase int

const (
	phasePreShoot penaltyPhase = iota // ball + goalie at rest, brief pause
	phaseShoot                        // ball flying, goalie diving
	phaseResult                       // SAVE or GOAL flash
)

// Orb (kept under the original name for back-compat) is a side-panel
// animation showing a penalty kick at goal with a diving goalie.
// Ball position uses one harmonica spring; goalie horizontal another.
type Orb struct {
	width  int // grid width
	height int // grid height

	// Reusable grid + style buffers (allocated once in NewOrb).
	grid  [][]rune
	style [][]int

	// Springs.
	ballXSpring harmonica.Spring
	ballYSpring harmonica.Spring
	keepSpring  harmonica.Spring

	// Current positions (float).
	ballX, ballY     float64
	ballVX, ballVY   float64
	keepX            float64
	keepVX           float64

	// Targets per shot.
	targetBallX, targetBallY float64
	targetKeepX              float64

	// Rest positions (penalty spot, center of goal).
	restBallX, restBallY float64
	restKeepX            float64

	phase    penaltyPhase
	phaseT   int // frame counter inside phase
	saved    bool
	totalShots int
	saves      int

	rng *rand.Rand
}

// NewOrb returns the penalty animation sized for the side panel.
// Width is generous so the GOAL!/SAVE! flash beside row h/2 fits
// without clipping against the rounded border.
func NewOrb() Orb {
	w, h := 38, 16
	grid := make([][]rune, h)
	style := make([][]int, h)
	for i := range grid {
		grid[i] = make([]rune, w)
		style[i] = make([]int, w)
	}
	o := Orb{
		width:       w,
		height:      h,
		grid:        grid,
		style:       style,
		ballXSpring: harmonica.NewSpring(harmonica.FPS(30), 6.0, 0.55),
		ballYSpring: harmonica.NewSpring(harmonica.FPS(30), 6.0, 0.55),
		keepSpring:  harmonica.NewSpring(harmonica.FPS(30), 5.0, 0.45),
		restBallX:   float64(w) / 2,
		restBallY:   float64(h) - 2,
		restKeepX:   float64(w) / 2,
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	o.ballX = o.restBallX
	o.ballY = o.restBallY
	o.keepX = o.restKeepX
	o.targetBallX = o.restBallX
	o.targetBallY = o.restBallY
	o.targetKeepX = o.restKeepX
	o.phase = phasePreShoot
	return o
}

func (o Orb) Init() tea.Cmd { return orbTick() }

// Update advances the animation by one frame.
func (o Orb) Update(msg tea.Msg) (Orb, tea.Cmd) {
	if _, ok := msg.(orbTickMsg); ok {
		o.phaseT++
		switch o.phase {
		case phasePreShoot:
			// 18 frames (~600ms) at rest, then aim.
			if o.phaseT > 18 {
				o.aim()
				o.phase = phaseShoot
				o.phaseT = 0
			}
		case phaseShoot:
			// Run springs.
			o.ballX, o.ballVX = o.ballXSpring.Update(o.ballX, o.ballVX, o.targetBallX)
			o.ballY, o.ballVY = o.ballYSpring.Update(o.ballY, o.ballVY, o.targetBallY)
			o.keepX, o.keepVX = o.keepSpring.Update(o.keepX, o.keepVX, o.targetKeepX)
			// 30 frames (~1s) of flight.
			if o.phaseT > 30 {
				o.judge()
				o.phase = phaseResult
				o.phaseT = 0
			}
		case phaseResult:
			// 24 frames of result, then reset.
			if o.phaseT > 24 {
				o.ballX = o.restBallX
				o.ballY = o.restBallY
				o.keepX = o.restKeepX
				o.ballVX, o.ballVY, o.keepVX = 0, 0, 0
				o.phase = phasePreShoot
				o.phaseT = 0
			}
		}
		return o, orbTick()
	}
	return o, nil
}

// aim picks a target corner for the ball + a guess for the goalie.
func (o *Orb) aim() {
	corners := []float64{4.5, float64(o.width) - 5.5}
	heights := []float64{1.5, 3.5}
	o.targetBallX = corners[o.rng.Intn(2)]
	o.targetBallY = heights[o.rng.Intn(2)]
	// Goalie picks one of two sides — 45% chance to match ball side.
	if o.rng.Float64() < 0.45 {
		o.targetKeepX = o.targetBallX
	} else {
		// dive opposite side
		if o.targetBallX < float64(o.width)/2 {
			o.targetKeepX = corners[1]
		} else {
			o.targetKeepX = corners[0]
		}
	}
}

// judge decides save vs goal based on goalie + ball proximity at end of flight.
func (o *Orb) judge() {
	// Save if goalie's x within 2.5 cells of ball x AND ball is in upper goal area.
	dx := math.Abs(o.keepX - o.ballX)
	o.saved = dx < 2.5 && o.ballY < 5
	o.totalShots++
	if o.saved {
		o.saves++
	}
}

// View renders the scene framed by a rounded box.
// Reuses pre-allocated grid + style slices, clearing them in place each frame.
func (o Orb) View() string {
	w, h := o.width, o.height
	grid := o.grid
	style := o.style
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			grid[y][x] = ' '
			style[y][x] = 0
		}
	}

	// Goal frame: top bar at row 0, side posts at col 3 and col w-4 down to row 4.
	goalLeft := 3
	goalRight := w - 4
	goalTop := 0
	goalBottom := 4
	for col := goalLeft; col <= goalRight; col++ {
		grid[goalTop][col] = '─'
		style[goalTop][col] = 1
	}
	grid[goalTop][goalLeft] = '┌'
	grid[goalTop][goalRight] = '┐'
	for row := goalTop + 1; row <= goalBottom; row++ {
		grid[row][goalLeft] = '│'
		style[row][goalLeft] = 1
		grid[row][goalRight] = '│'
		style[row][goalRight] = 1
	}

	// Net cross-hatch inside goal.
	for row := goalTop + 1; row <= goalBottom-1; row++ {
		for col := goalLeft + 1; col <= goalRight-1; col++ {
			if (row+col)%3 == 0 {
				grid[row][col] = '·'
				style[row][col] = 4
			}
		}
	}

	// Penalty spot mark.
	psRow, psCol := h-2, w/2
	if psRow >= 0 && psRow < h && psCol >= 0 && psCol < w {
		grid[psRow][psCol] = '×'
		style[psRow][psCol] = 4
	}

	// Goalie: stylized as 'Ó' (with arms during dive).
	keeperRow := goalBottom - 1
	keeperCol := int(math.Round(o.keepX))
	if keeperCol < goalLeft+1 {
		keeperCol = goalLeft + 1
	}
	if keeperCol > goalRight-1 {
		keeperCol = goalRight - 1
	}
	if keeperRow >= 0 && keeperRow < h && keeperCol >= 0 && keeperCol < w {
		// Arm characters depending on dive direction.
		left, right := keeperCol-1, keeperCol+1
		armChar := '─'
		if math.Abs(o.keepX-o.restKeepX) > 1.5 {
			armChar = '═'
		}
		if left > goalLeft && o.phase == phaseShoot {
			grid[keeperRow][left] = armChar
			style[keeperRow][left] = 2
		}
		if right < goalRight && o.phase == phaseShoot {
			grid[keeperRow][right] = armChar
			style[keeperRow][right] = 2
		}
		grid[keeperRow][keeperCol] = '◇'
		style[keeperRow][keeperCol] = 2
	}

	// Ball: ● glyph, big when at rest, smaller in flight.
	bRow := clampInt(int(math.Round(o.ballY)), 0, h-1)
	bCol := clampInt(int(math.Round(o.ballX)), 0, w-1)
	{
		ballChar := '●'
		if o.phase == phaseShoot {
			ballChar = '◉'
		}
		grid[bRow][bCol] = ballChar
		style[bRow][bCol] = 3
	}

	// Compose with styling.
	var sb strings.Builder
	resultText := ""
	if o.phase == phaseResult {
		if o.saved {
			resultText = StyleOK.Bold(true).Render("  SAVE!")
		} else {
			resultText = StyleErr.Bold(true).Render("  GOAL!")
		}
	}

	for y, row := range grid {
		for x, r := range row {
			s := penaltyStyle(style[y][x])
			sb.WriteString(s.Render(string(r)))
			_ = x
		}
		if y == 0 && o.totalShots > 0 {
			sb.WriteString("  " + StyleDim.Render(scoreLine(o)))
		}
		if y == h/2 && resultText != "" {
			sb.WriteString(resultText)
		}
		if y < h-1 {
			sb.WriteString("\n")
		}
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 2)
	return box.Render(sb.String())
}

func scoreLine(o Orb) string {
	goals := o.totalShots - o.saves
	return fmt.Sprintf("saves %d · goals %d", o.saves, goals)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// penaltyStyle indexes into a small style table for grid characters.
func penaltyStyle(idx int) lipgloss.Style {
	switch idx {
	case 1: // goal frame
		return lipgloss.NewStyle().Foreground(colorMuted)
	case 2: // goalie
		return StyleHi
	case 3: // ball
		return StyleOK.Bold(true)
	case 4: // net + penalty spot
		return StyleDim
	}
	return lipgloss.NewStyle()
}
