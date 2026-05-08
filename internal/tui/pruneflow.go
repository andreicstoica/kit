package tui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type pruneStage int

const (
	pruneStageSelect pruneStage = iota
	pruneStageConfirm
	pruneStageRun
	pruneStageDone
	pruneStageAborted
)

type pruneModel struct {
	layout     liftoff.Layout
	stage      pruneStage
	candidates []liftoff.PruneCandidate
	selected   map[int]bool
	cursor     int

	results map[string]string // name -> status string
	spinner spinner.Model
	failed  bool

	width, height int
}

// NewPruneModel constructs the prune flow.
func NewPruneModel(layout liftoff.Layout) (tea.Model, error) {
	cands, err := layout.FindMergedWorktrees()
	if err != nil {
		return nil, err
	}
	if len(cands) == 0 {
		return nil, errors.New("no stale worktrees found")
	}
	sel := map[int]bool{}
	for i := range cands {
		sel[i] = true // default-select all
	}
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorAccent)
	return &pruneModel{
		layout:     layout,
		stage:      pruneStageSelect,
		candidates: cands,
		selected:   sel,
		results:    map[string]string{},
		spinner:    sp,
	}, nil
}

func (m *pruneModel) Init() tea.Cmd { return m.spinner.Tick }

type pruneRunMsg struct {
	name   string
	err    error
	done   bool
}

func (m *pruneModel) startRun() tea.Cmd {
	return func() tea.Msg {
		// One iteration per call — chain by repeatedly returning a message
		// that triggers the next. Simpler: do them all here and emit once.
		// (Prune is expected to be small; sequential blocking is fine.)
		for i, c := range m.candidates {
			if !m.selected[i] {
				continue
			}
			err := pruneOne(m.layout, c)
			status := "removed"
			if err != nil {
				status = "failed: " + err.Error()
			}
			m.results[c.Name] = status
		}
		return pruneRunMsg{done: true}
	}
}

func pruneOne(layout liftoff.Layout, c liftoff.PruneCandidate) error {
	// Stop services first.
	st, _ := liftoff.LoadState()
	if st != nil {
		if meta, ok := st.Worktrees[c.Name]; ok {
			ports := liftoff.PortsForSlot(meta.Slot)
			for _, svc := range liftoff.AllServices {
				if liftoff.StatusOf(c.Name, svc, ports).Alive {
					_ = liftoff.StopService(c.Name, svc)
				}
			}
		}
	}
	// Remove worktree + branch.
	if err := layout.RemoveWorktree(c.Path, nil); err != nil {
		return err
	}
	_ = layout.DeleteBranch(c.Branch, nil)
	// Free slot + clean gtab.
	if st != nil {
		st.FreeSlot(c.Name)
		_ = st.Save()
	}
	_ = layout.RemoveGtab(c.Name)
	return nil
}

func (m *pruneModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.stage = pruneStageAborted
			return m, tea.Quit
		}
	case pruneRunMsg:
		if msg.done {
			m.stage = pruneStageDone
			return m, nil
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.stage == pruneStageRun {
			return m, cmd
		}
	}

	switch m.stage {
	case pruneStageSelect:
		return m.updateSelect(msg)
	case pruneStageConfirm:
		return m.updateConfirm(msg)
	case pruneStageDone, pruneStageAborted:
		if k, ok := msg.(tea.KeyMsg); ok {
			if k.Type == tea.KeyEnter || k.Type == tea.KeyEsc || k.String() == "q" {
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m *pruneModel) updateSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.candidates)-1 {
				m.cursor++
			}
		case " ", "tab":
			m.selected[m.cursor] = !m.selected[m.cursor]
		case "a":
			for i := range m.candidates {
				m.selected[i] = true
			}
		case "n":
			for i := range m.candidates {
				m.selected[i] = false
			}
		case "enter":
			anySelected := false
			for _, v := range m.selected {
				if v {
					anySelected = true
					break
				}
			}
			if anySelected {
				m.stage = pruneStageConfirm
			}
		case "esc":
			m.stage = pruneStageAborted
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *pruneModel) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "y", "Y", "enter":
			m.stage = pruneStageRun
			return m, tea.Batch(m.spinner.Tick, m.startRun())
		case "n", "N", "esc":
			m.stage = pruneStageAborted
			return m, tea.Quit
		case "backspace":
			m.stage = pruneStageSelect
		}
	}
	return m, nil
}

func (m *pruneModel) View() string {
	switch m.stage {
	case pruneStageSelect:
		return m.viewSelect()
	case pruneStageConfirm:
		return m.viewConfirm()
	case pruneStageRun:
		return StyleTitle.Render("kit prune — running") + "\n\n  " + m.spinner.View() + " washing selected worktrees…"
	case pruneStageDone:
		return m.viewDone()
	case pruneStageAborted:
		return StyleWarn.Render("aborted.\n")
	}
	return ""
}

func (m *pruneModel) viewSelect() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("kit prune — pick stale worktrees") + "\n\n")
	for i, c := range m.candidates {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		box := "[ ]"
		if m.selected[i] {
			box = StyleOK.Render("[x]")
		}
		emoji := liftoff.EmojiFor(c.Name)
		if emoji != "" {
			emoji += " "
		}
		b.WriteString(fmt.Sprintf("%s%s %s%s  %s\n", cursor, box, emoji, c.Name, StyleDim.Render("("+c.Reason+")")))
	}
	b.WriteString("\n" + StyleHelp.Render("space toggle · a all · n none · enter continue · esc abort"))
	return b.String()
}

func (m *pruneModel) viewConfirm() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("kit prune — confirm") + "\n\n")
	count := 0
	for i, c := range m.candidates {
		if !m.selected[i] {
			continue
		}
		count++
		b.WriteString("  " + StyleErr.Render("✗") + " " + c.Name + StyleDim.Render(" — "+c.Reason) + "\n")
	}
	b.WriteString(fmt.Sprintf("\nremove %d worktrees? this stops services, deletes worktree dirs, and removes branches.\n\n", count))
	b.WriteString(StyleHelp.Render("[Y]es · [n] cancel · backspace back · esc abort"))
	return b.String()
}

func (m *pruneModel) viewDone() string {
	var b strings.Builder
	b.WriteString(StyleOK.Render("✓ kit prune complete") + "\n\n")
	for name, status := range m.results {
		marker := StyleOK.Render(Glyph("done"))
		if strings.HasPrefix(status, "failed") {
			marker = StyleErr.Render(Glyph("failed"))
		}
		b.WriteString(fmt.Sprintf("  %s  %s — %s\n", marker, name, status))
	}
	b.WriteString("\n" + StyleHelp.Render("press enter to exit"))
	return b.String()
}

// RunPruneTUI is the cobra entry point.
func RunPruneTUI(layout liftoff.Layout) error {
	m, err := NewPruneModel(layout)
	if err != nil {
		return err
	}
	_, runErr := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return runErr
}
