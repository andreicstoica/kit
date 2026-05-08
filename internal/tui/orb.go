package tui

import (
	"math"
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

// Orb is a small animated panel: 3 dots orbiting a central pulse.
// Used to give kit dress some life while the worktree is being built.
type Orb struct {
	width  int
	height int

	angle      float64
	pulseSpring harmonica.Spring
	pulseValue  float64
	pulseVel    float64
	pulseTarget float64

	frame int
}

// NewOrb returns an Orb sized for the side panel.
func NewOrb() Orb {
	return Orb{
		width:       28,
		height:      14,
		angle:       0,
		pulseSpring: harmonica.NewSpring(harmonica.FPS(30), 4.0, 0.4),
		pulseValue:  0.5,
		pulseTarget: 1.0,
	}
}

// Init returns the first tick.
func (o Orb) Init() tea.Cmd { return orbTick() }

// Update advances the animation on each tick.
func (o Orb) Update(msg tea.Msg) (Orb, tea.Cmd) {
	if _, ok := msg.(orbTickMsg); ok {
		o.angle += 0.12 // ~7°/frame
		if o.angle > 2*math.Pi {
			o.angle -= 2 * math.Pi
		}
		// Pulse intensity oscillates between two targets.
		o.frame++
		if o.frame%30 == 0 {
			if o.pulseTarget > 0.7 {
				o.pulseTarget = 0.3
			} else {
				o.pulseTarget = 1.0
			}
		}
		o.pulseValue, o.pulseVel = o.pulseSpring.Update(o.pulseValue, o.pulseVel, o.pulseTarget)
		return o, orbTick()
	}
	return o, nil
}

// View renders the orb as a 28×14 character grid framed by a rounded box.
func (o Orb) View() string {
	w, h := o.width, o.height
	grid := make([][]rune, h)
	for i := range grid {
		grid[i] = make([]rune, w)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}

	cx, cy := w/2, h/2

	// Center pulse — radius derived from spring value.
	radiusF := 1.0 + o.pulseValue*1.5
	pulseRadius := radiusF
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Aspect-correct distance (chars are ~2x taller than wide).
			dx := float64(x - cx)
			dy := float64(y-cy) * 2
			d := math.Sqrt(dx*dx + dy*dy)
			if d <= pulseRadius {
				grid[y][x] = '·'
			}
			if d <= pulseRadius-0.6 {
				grid[y][x] = '•'
			}
			if d <= pulseRadius-1.2 {
				grid[y][x] = '●'
			}
		}
	}

	// Three orbiting dots at angles theta, theta+120°, theta+240°.
	orbitR := float64(min(w, h)) * 0.65
	for k := 0; k < 3; k++ {
		theta := o.angle + float64(k)*2*math.Pi/3
		ox := float64(cx) + math.Cos(theta)*orbitR/2
		oy := float64(cy) + math.Sin(theta)*orbitR/4
		ix, iy := int(math.Round(ox)), int(math.Round(oy))
		if iy >= 0 && iy < h && ix >= 0 && ix < w {
			grid[iy][ix] = '◌'
		}
	}

	var sb strings.Builder
	for y, row := range grid {
		// Color the central pulse vs orbits differently by character.
		var line strings.Builder
		for _, r := range row {
			switch r {
			case '●':
				line.WriteString(StyleHi.Render(string(r)))
			case '•':
				line.WriteString(StyleOK.Render(string(r)))
			case '·':
				line.WriteString(StyleDim.Render(string(r)))
			case '◌':
				line.WriteString(StyleHi.Render(string(r)))
			default:
				line.WriteString(" ")
			}
		}
		sb.WriteString(line.String())
		if y < h-1 {
			sb.WriteString("\n")
		}
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(0, 1).
		Width(w + 2)
	return box.Render(sb.String())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
