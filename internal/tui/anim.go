package tui

import (
	"math"
	"math/rand"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/lipgloss"
)

// animTickMsg is the shared ~30fps frame tick for every animation.
type animTickMsg time.Time

func animTick() tea.Cmd {
	return tea.Tick(33*time.Millisecond, func(t time.Time) tea.Msg {
		return animTickMsg(t)
	})
}

// Animation is a side-panel animation shown during worktree setup. Each draws
// into a fixed-size canvas per frame, so the box never changes size.
type Animation interface {
	Init() tea.Cmd
	Update(tea.Msg) (Animation, tea.Cmd)
	View() string
}

// animConstructors is the pool NewAnimation picks from at random. Add an entry
// to ship a new animation.
var animConstructors = []func(*rand.Rand) Animation{
	newSoccer,
	newBasketball,
	newRace,
	newRocket,
	newFactory,
}

// NewAnimation returns a random animation from the pool.
func NewAnimation() Animation {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	return animConstructors[rng.Intn(len(animConstructors))](rng)
}

// Shared box size, so every animation renders identically.
const (
	animWidth  = 38
	animHeight = 16
)

// Shared style indices, so all animations route through one palette.
const (
	stClear = iota // 0 — background
	stFrame        // structure: posts, rims, belts, ground
	stActor        // the diving/running/working actor (bright accent)
	stBall         // the moving object (ball, package, rocket)
	stDim          // faint detail: nets, stars, marks, trails
	stOK           // success flash
	stErr          // failure flash
	stWarn         // secondary highlight (flames, flags, stamps)
)

// animPalette maps a style index to a lipgloss style. Shared by all animations.
func animPalette(idx int) lipgloss.Style {
	switch idx {
	case stFrame:
		return lipgloss.NewStyle().Foreground(colorMuted)
	case stActor:
		return StyleHi
	case stBall:
		return StyleOK.Bold(true)
	case stDim:
		return StyleDim
	case stOK:
		return StyleOK.Bold(true)
	case stErr:
		return StyleErr.Bold(true)
	case stWarn:
		return StyleWarn.Bold(true)
	}
	return lipgloss.NewStyle()
}

// canvas is a fixed-size rune grid plus a parallel style-index grid. Drawing
// everything into it — including text and footers — keeps the output exactly
// w×h, so the framed box never grows or shrinks.
type canvas struct {
	w, h  int
	grid  [][]rune
	style [][]int
}

func newCanvas(w, h int) canvas {
	grid := make([][]rune, h)
	style := make([][]int, h)
	for i := range grid {
		grid[i] = make([]rune, w)
		style[i] = make([]int, w)
	}
	c := canvas{w: w, h: h, grid: grid, style: style}
	c.clear()
	return c
}

func (c *canvas) clear() {
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			c.grid[y][x] = ' '
			c.style[y][x] = stClear
		}
	}
}

// set draws one styled rune, ignoring out-of-bounds coordinates.
func (c *canvas) set(x, y int, r rune, style int) {
	if x < 0 || x >= c.w || y < 0 || y >= c.h {
		return
	}
	c.grid[y][x] = r
	c.style[y][x] = style
}

// setf is set with float positions rounded to the nearest cell.
func (c *canvas) setf(x, y float64, r rune, style int) {
	c.set(int(math.Round(x)), int(math.Round(y)), r, style)
}

// text draws s left-to-right from (x,y), one rune per cell, all one style.
func (c *canvas) text(x, y int, s string, style int) {
	for i, r := range []rune(s) {
		c.set(x+i, y, r, style)
	}
}

// textCenter centers s on row y, anchored on column w/2 so it lines up with
// markers drawn there (ball, penalty spot, etc.).
func (c *canvas) textCenter(y int, s string, style int) {
	c.text(c.w/2-len([]rune(s))/2, y, s, style)
}

// hline draws a horizontal run of r from x0 to x1 inclusive.
func (c *canvas) hline(x0, x1, y int, r rune, style int) {
	for x := x0; x <= x1; x++ {
		c.set(x, y, r, style)
	}
}

// vline draws a vertical run of r from y0 to y1 inclusive.
func (c *canvas) vline(x, y0, y1 int, r rune, style int) {
	for y := y0; y <= y1; y++ {
		c.set(x, y, r, style)
	}
}

// render composes the grid through animPalette and frames it in the shared box.
func (c canvas) render() string {
	var sb strings.Builder
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			sb.WriteString(animPalette(c.style[y][x]).Render(string(c.grid[y][x])))
		}
		if y < c.h-1 {
			sb.WriteByte('\n')
		}
	}
	return animBox.Render(sb.String())
}

// animBox is the shared rounded frame around every animation.
var animBox = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(colorAccent).
	Padding(1, 3)

// spring1D pairs a harmonica spring with its position + velocity so every
// animation drives motion the same way: build one, then call to(target) each
// frame. Shared so soccer, basketball, race, and rocket all move on the same
// physics instead of a mix of springs, lerps, and hand-integrated steps.
type spring1D struct {
	s        harmonica.Spring
	pos, vel float64
}

// newSpring1D builds a spring at the shared 30fps step starting at pos. freq is
// the angular frequency (higher = snappier); damping is the ratio (1.0 =
// critical, <1 overshoots, >1 sluggish).
func newSpring1D(pos, freq, damping float64) spring1D {
	return spring1D{s: harmonica.NewSpring(harmonica.FPS(30), freq, damping), pos: pos}
}

// to advances the spring one frame toward target and returns the new position.
func (sp *spring1D) to(target float64) float64 {
	sp.pos, sp.vel = sp.s.Update(sp.pos, sp.vel, target)
	return sp.pos
}

// set hard-resets the position and zeroes velocity (used when looping to rest).
func (sp *spring1D) set(pos float64) { sp.pos, sp.vel = pos, 0 }

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
