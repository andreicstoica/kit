package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// washItem is a list entry representing one removable worktree.
type washItem struct {
	name     string
	path     string
	branch   string
	dirty    bool
	hasDB    bool
	hasGtab  bool
	isLegacy bool
}

func (w washItem) Title() string {
	t := w.name
	if w.isLegacy {
		t = StyleDim.Render("(legacy) ") + t
	}
	if w.dirty {
		t += "  " + StyleWarn.Render("● dirty")
	}
	return t
}
func (w washItem) Description() string {
	bits := []string{w.path}
	if w.branch != "" && w.branch != w.name {
		bits = append(bits, "branch="+w.branch)
	}
	tags := []string{}
	if w.hasDB {
		tags = append(tags, "db")
	}
	if w.hasGtab {
		tags = append(tags, "gtab")
	}
	if len(tags) > 0 {
		bits = append(bits, "["+strings.Join(tags, " ")+"]")
	}
	return StyleDim.Render(strings.Join(bits, "  ·  "))
}
func (w washItem) FilterValue() string { return w.name }

type washStage int

const (
	washStageSelect washStage = iota
	washStageConfirm
	washStageRunning
	washStageDone
	washStageAborted
)

type washModel struct {
	layout       liftoff.Layout
	stage        washStage
	list         list.Model
	selected     washItem
	dropDB       bool
	removeGtab   bool
	confirmCursor int

	spinner      spinner.Model
	updates      <-chan liftoff.StepUpdate
	stepTitles   []string
	stepStatuses []liftoff.StepStatus
	stepLines    map[int][]string
	failed       bool
	failureErr   error

	width, height int
}

func NewWashModel(layout liftoff.Layout) (tea.Model, error) {
	wts, err := layout.ListWorktrees()
	if err != nil {
		return nil, err
	}
	items := []list.Item{}
	for _, wt := range wts {
		if wt.IsMaster(layout) || wt.Bare {
			continue
		}
		name := wt.Name()
		items = append(items, washItem{
			name:     name,
			path:     wt.Path,
			branch:   wt.Branch,
			dirty:    liftoff.IsDirty(wt.Path),
			hasDB:    liftoff.HasPostgres() && liftoff.HasDB(name),
			hasGtab:  layout.HasGtab(name),
			isLegacy: wt.HasLegacyPrefix(),
		})
	}
	if len(items) == 0 {
		return nil, errors.New("no removable worktrees found")
	}
	dlg := list.NewDefaultDelegate()
	dlg.Styles.SelectedTitle = dlg.Styles.SelectedTitle.Foreground(colorAccent).BorderForeground(colorAccent)
	dlg.Styles.SelectedDesc = dlg.Styles.SelectedDesc.Foreground(colorAccent).BorderForeground(colorAccent)
	l := list.New(items, dlg, 0, 0)
	l.Title = "kit wash — pick a kit to strip"
	l.Styles.Title = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	l.SetShowHelp(true)
	l.SetFilteringEnabled(true)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorAccent)

	return &washModel{
		layout:     layout,
		stage:      washStageSelect,
		list:       l,
		dropDB:     true,
		removeGtab: true,
		spinner:    sp,
		stepLines:  map[int][]string{},
	}, nil
}

func (m *washModel) Init() tea.Cmd { return m.spinner.Tick }

func (m *washModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.list.SetSize(msg.Width, msg.Height-2)
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.stage = washStageAborted
			return m, tea.Quit
		}
	}
	switch m.stage {
	case washStageSelect:
		return m.updateSelect(msg)
	case washStageConfirm:
		return m.updateConfirm(msg)
	case washStageRunning:
		return m.updateRunning(msg)
	case washStageDone, washStageAborted:
		if k, ok := msg.(tea.KeyMsg); ok {
			if k.Type == tea.KeyEnter || k.Type == tea.KeyEsc || k.String() == "q" {
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m *washModel) updateSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "enter":
			if it, ok := m.list.SelectedItem().(washItem); ok {
				m.selected = it
				if !it.hasDB {
					m.dropDB = false
				}
				if !it.hasGtab {
					m.removeGtab = false
				}
				m.stage = washStageConfirm
				return m, nil
			}
		case "esc":
			m.stage = washStageAborted
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *washModel) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "up", "k":
			if m.confirmCursor > 0 {
				m.confirmCursor--
			}
		case "down", "j":
			if m.confirmCursor < 1 {
				m.confirmCursor++
			}
		case " ", "tab":
			switch m.confirmCursor {
			case 0:
				if m.selected.hasDB {
					m.dropDB = !m.dropDB
				}
			case 1:
				if m.selected.hasGtab {
					m.removeGtab = !m.removeGtab
				}
			}
		case "enter":
			m.stage = washStageRunning
			plan := liftoff.WashPlan{
				Name:         m.selected.name,
				WorktreePath: m.selected.path,
				DropDB:       m.dropDB,
				RemoveGtab:   m.removeGtab,
			}
			m.stepTitles = washStepTitles(plan)
			m.stepStatuses = make([]liftoff.StepStatus, len(m.stepTitles))
			m.updates = m.layout.RunWash(plan)
			return m, tea.Batch(m.spinner.Tick, washNextUpdate(m.updates))
		case "esc":
			m.stage = washStageAborted
			return m, tea.Quit
		case "backspace":
			m.stage = washStageSelect
		}
	}
	return m, nil
}

func washStepTitles(p liftoff.WashPlan) []string {
	titles := []string{
		"remove worktree " + p.WorktreePath,
		"delete branch " + p.Name,
	}
	if p.DropDB {
		titles = append(titles, "drop database "+liftoff.DBName(p.Name))
	} else {
		titles = append(titles, "drop database (skipped)")
	}
	if p.RemoveGtab {
		titles = append(titles, "remove gtab workspace")
	} else {
		titles = append(titles, "remove gtab (skipped)")
	}
	return titles
}

type washStepMsg struct {
	upd liftoff.StepUpdate
	ok  bool
}

func washNextUpdate(ch <-chan liftoff.StepUpdate) tea.Cmd {
	return func() tea.Msg {
		u, ok := <-ch
		return washStepMsg{upd: u, ok: ok}
	}
}

func (m *washModel) updateRunning(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case washStepMsg:
		if !msg.ok {
			m.stage = washStageDone
			return m, nil
		}
		u := msg.upd
		if u.Index >= 0 && u.Index < len(m.stepStatuses) {
			m.stepStatuses[u.Index] = u.Status
			if u.Line != "" {
				m.stepLines[u.Index] = append(m.stepLines[u.Index], u.Line)
			}
			if u.Status == liftoff.StepFailed && u.Index < 2 {
				m.failed = true
				m.failureErr = u.Err
			}
		}
		return m, washNextUpdate(m.updates)
	}
	return m, nil
}

func (m *washModel) View() string {
	switch m.stage {
	case washStageSelect:
		return m.list.View()
	case washStageConfirm:
		return m.viewConfirm()
	case washStageRunning:
		return m.viewRunning()
	case washStageDone:
		return m.viewDone()
	case washStageAborted:
		return StyleWarn.Render("aborted.\n")
	}
	return ""
}

func (m *washModel) viewConfirm() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("kit wash — "+m.selected.name) + "\n\n")
	b.WriteString("worktree: " + m.selected.path + "\n")
	b.WriteString("branch:   " + m.selected.branch + "\n")
	if m.selected.dirty {
		b.WriteString(StyleWarn.Render("⚠ uncommitted changes will be lost") + "\n")
	}
	b.WriteString("\n")
	checks := []struct {
		label    string
		on       bool
		disabled bool
	}{
		{"drop database " + liftoff.DBName(m.selected.name), m.dropDB, !m.selected.hasDB},
		{"remove gtab workspace " + m.selected.name + ".applescript", m.removeGtab, !m.selected.hasGtab},
	}
	for i, c := range checks {
		cursor := "  "
		if m.confirmCursor == i {
			cursor = "> "
		}
		box := "[ ]"
		if c.on {
			box = StyleOK.Render("[x]")
		}
		if c.disabled {
			box = StyleDim.Render("[-]")
		}
		b.WriteString(cursor + box + " " + c.label + "\n")
	}
	b.WriteString("\n" + StyleHelp.Render("space: toggle · enter: confirm · backspace: back · esc: abort"))
	return b.String()
}

func (m *washModel) viewRunning() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("kit wash — "+m.selected.name) + "\n\n")
	for i, t := range m.stepTitles {
		var marker string
		switch m.stepStatuses[i] {
		case liftoff.StepRunning:
			marker = m.spinner.View()
		case liftoff.StepDone:
			marker = StyleOK.Render(Glyph("done"))
		case liftoff.StepSkipped:
			marker = StyleDim.Render(Glyph("skipped"))
		case liftoff.StepFailed:
			marker = StyleErr.Render(Glyph("failed"))
		default:
			marker = StyleDim.Render(Glyph("pending"))
		}
		b.WriteString(fmt.Sprintf("  %s  %s\n", marker, t))
	}
	if m.failed {
		b.WriteString("\n" + StyleErr.Render("failed: "+m.failureErr.Error()) + "\n")
	}
	return b.String()
}

func (m *washModel) viewDone() string {
	if m.failed {
		return StyleErr.Render("✗ kit wash failed: "+m.failureErr.Error()) + "\n\n" +
			StyleHelp.Render("press enter to exit")
	}
	var b strings.Builder
	b.WriteString(StyleOK.Render("✓ "+m.selected.name+" washed.") + "\n")
	b.WriteString("\n" + StyleHelp.Render("press enter to exit"))
	return b.String()
}

// RunWashTUI is the cobra entry point.
func RunWashTUI(layout liftoff.Layout) error {
	if !layout.MasterIsRepo() {
		return fmt.Errorf("master repo not found at %s", layout.Master)
	}
	m, err := NewWashModel(layout)
	if err != nil {
		return err
	}
	final, runErr := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if runErr != nil {
		return runErr
	}
	if wm, ok := final.(*washModel); ok && wm.failed {
		return errors.New("kit wash reported a failure")
	}
	_ = time.Second
	return nil
}
