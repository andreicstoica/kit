package tui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type mergedStage int

const (
	mergedStageSelect mergedStage = iota
	mergedStageConfirm
	mergedStageRun
	mergedStageDone
	mergedStageAborted
)

type mergedModel struct {
	layout     liftoff.Layout
	stage      mergedStage
	candidates []liftoff.MergedCandidate
	selected   map[int]bool
	cursor     int

	results map[string]string // name -> status string
	spinner spinner.Model
	help    help.Model
	keys    KeyMap
	failed  bool

	width, height int
}

// newMergedModel constructs the merged-wash flow.
func newMergedModel(layout liftoff.Layout) (tea.Model, error) {
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
	return &mergedModel{
		layout:     layout,
		stage:      mergedStageSelect,
		candidates: cands,
		selected:   sel,
		results:    map[string]string{},
		spinner:    sp,
		help:       NewHelp(),
		keys:       DefaultKeymap,
	}, nil
}

func (m *mergedModel) Init() tea.Cmd { return m.spinner.Tick }

type mergedRunMsg struct {
	name   string
	err    error
	done   bool
}

func (m *mergedModel) startRun() tea.Cmd {
	return func() tea.Msg {
		// One iteration per call — chain by repeatedly returning a message
		// that triggers the next. Simpler: do them all here and emit once.
		// (Merged-wash is expected to be small; sequential blocking is fine.)
		for i, c := range m.candidates {
			if !m.selected[i] {
				continue
			}
			err := mergedWashOne(m.layout, c)
			status := "removed"
			if err != nil {
				status = "failed: " + err.Error()
			}
			m.results[c.Name] = status
		}
		return mergedRunMsg{done: true}
	}
}

func mergedWashOne(layout liftoff.Layout, c liftoff.MergedCandidate) error {
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
	_ = liftoff.WithConfigLock(func(cfg *liftoff.Config) error {
		cfg.FreeSlot(c.Name)
		return nil
	})
	_ = layout.RemoveGtab(c.Name)
	return nil
}

func (m *mergedModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = msg.Width
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.stage = mergedStageAborted
			return m, tea.Quit
		}
		if msg.String() == "?" {
			m.help.ShowAll = !m.help.ShowAll
		}
	case mergedRunMsg:
		if msg.done {
			m.stage = mergedStageDone
			return m, nil
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.stage == mergedStageRun {
			return m, cmd
		}
	}

	switch m.stage {
	case mergedStageSelect:
		return m.updateSelect(msg)
	case mergedStageConfirm:
		return m.updateConfirm(msg)
	case mergedStageDone, mergedStageAborted:
		if k, ok := msg.(tea.KeyMsg); ok {
			if k.Type == tea.KeyEnter || k.Type == tea.KeyEsc || k.String() == "q" {
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m *mergedModel) updateSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
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
				m.stage = mergedStageConfirm
			}
		case "esc":
			m.stage = mergedStageAborted
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *mergedModel) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "y", "Y", "enter":
			m.stage = mergedStageRun
			return m, tea.Batch(m.spinner.Tick, m.startRun())
		case "n", "N", "esc":
			m.stage = mergedStageAborted
			return m, tea.Quit
		case "backspace":
			m.stage = mergedStageSelect
		}
	}
	return m, nil
}

func (m *mergedModel) View() string {
	var body string
	switch m.stage {
	case mergedStageSelect:
		body = m.viewSelect()
	case mergedStageConfirm:
		body = m.viewConfirm()
	case mergedStageRun:
		body = StyleTitle.Render("kit wash --merged — running") + "\n\n  " + m.spinner.View() + " washing selected worktrees…"
	case mergedStageDone:
		body = m.viewDone()
	case mergedStageAborted:
		return StyleWarn.Render("aborted.\n")
	}
	return body + "\n" + m.help.View(m.keys)
}

func (m *mergedModel) viewSelect() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("kit wash --merged — pick stale worktrees") + "\n\n")
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

func (m *mergedModel) viewConfirm() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("kit wash --merged — confirm") + "\n\n")
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

func (m *mergedModel) viewDone() string {
	var b strings.Builder
	b.WriteString(StyleOK.Render("✓ kit wash --merged complete") + "\n\n")
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

// RunMergedWashTUI is the cobra entry point.
func RunMergedWashTUI(layout liftoff.Layout) error {
	m, err := newMergedModel(layout)
	if err != nil {
		return err
	}
	_, runErr := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return runErr
}
