package tui

import (
	"math/rand"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// animGalleryNames labels each tile, in animConstructors order.
var animGalleryNames = []string{"soccer", "basketball", "race", "rocket", "factory"}

// animGallery tiles one of every animation, all driven off the same tick.
type animGallery struct {
	anims []Animation
	w     int
}

func newAnimGallery() animGallery {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	g := animGallery{}
	for _, mk := range animConstructors {
		g.anims = append(g.anims, mk(rng))
	}
	return g
}

func (g animGallery) Init() tea.Cmd { return animTick() }

func (g animGallery) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		g.w = msg.Width
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return g, tea.Quit
		}
	case animTickMsg:
		// Each Update returns its own tick, but they share one tick type —
		// collapse to a single tick so the frame rate doesn't compound per tile.
		for i, a := range g.anims {
			g.anims[i], _ = a.Update(msg)
		}
		return g, animTick()
	}
	return g, nil
}

func (g animGallery) View() string {
	tiles := make([]string, len(g.anims))
	for i, a := range g.anims {
		label := lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render(animGalleryNames[i])
		tiles[i] = lipgloss.JoinVertical(lipgloss.Center, label, a.View())
	}

	// Wrap tiles into rows that fit the terminal width.
	tileW := lipgloss.Width(tiles[0]) + 2
	perRow := 1
	if g.w > tileW {
		perRow = g.w / tileW
	}
	var rows []string
	for i := 0; i < len(tiles); i += perRow {
		end := min(i+perRow, len(tiles))
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, tiles[i:end]...))
	}
	rows = append(rows, StyleDim.Render("press q to quit"))
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// RunAnimGallery previews every animation at once. Quits on q / esc / ctrl-c.
func RunAnimGallery() error {
	_, err := tea.NewProgram(newAnimGallery(), tea.WithAltScreen()).Run()
	return err
}
