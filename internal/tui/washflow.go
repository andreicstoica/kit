package tui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// washItem is a list entry representing one removable worktree.
type washItem struct {
	name       string
	emoji      string
	path       string
	branch     string
	dirty      bool
	aheadCount int // commits in HEAD not yet in origin/<main> — lost on -D
	hasDB      bool
	hasGtab    bool
	isLegacy   bool
	displayIdx int // 1-based for numeric quick-pick
}

func (w washItem) Title() string {
	t := w.name
	if w.emoji != "" {
		t = w.emoji + " " + t
	}
	if w.displayIdx > 0 && w.displayIdx < 10 {
		t = StyleHi.Render(fmt.Sprintf("%d ", w.displayIdx)) + t
	}
	if w.isLegacy {
		t += "  " + StyleDim.Render("(legacy)")
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
	layout        liftoff.Layout
	stage         washStage
	list          list.Model
	selected      washItem
	dropDB        bool
	removeGtab    bool
	confirmCursor int
	confirmArmed  bool // second-confirm latch when the branch has unmerged work

	spinner      spinner.Model
	help         help.Model
	keys         KeyMap
	updates      <-chan liftoff.StepUpdate
	stepTitles   []string
	stepStatuses []liftoff.StepStatus
	stepLines    map[int][]string
	failed       bool
	failureErr   error

	width, height int
}

func NewWashModel(layout liftoff.Layout) (tea.Model, error) {
	return NewWashModelFor(layout, "")
}

// NewWashModelFor builds the wash model. If preselected is non-empty and
// matches a worktree, the picker stage is skipped and the model jumps
// straight to the confirm screen for that worktree.
func NewWashModelFor(layout liftoff.Layout, preselected string) (tea.Model, error) {
	wts, err := layout.ListWorktrees()
	if err != nil {
		return nil, err
	}
	items := []list.Item{}
	var preselectedItem *washItem
	for _, wt := range wts {
		if wt.IsMaster(layout) || wt.Bare {
			continue
		}
		name := wt.Name()
		ahead, _ := layout.AheadBehind(wt.Path)
		it := washItem{
			name:       name,
			emoji:      liftoff.EmojiFor(name),
			path:       wt.Path,
			branch:     wt.Branch,
			dirty:      liftoff.IsDirty(wt.Path),
			aheadCount: ahead,
			hasDB:      liftoff.HasPostgres() && liftoff.HasDB(name),
			hasGtab:    layout.HasGtab(name),
			isLegacy:   wt.HasLegacyPrefix(),
			displayIdx: len(items) + 1,
		}
		items = append(items, it)
		if preselected != "" && name == preselected {
			pinned := it
			preselectedItem = &pinned
		}
	}
	if len(items) == 0 {
		return nil, errors.New("no removable worktrees found")
	}
	l := list.New(items, NewListDelegate(), 0, 0)
	StyleList(&l, "kit wash — pick a kit to strip", true)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorAccent)

	m := &washModel{
		layout:     layout,
		stage:      washStageSelect,
		list:       l,
		dropDB:     true,
		removeGtab: true,
		spinner:    sp,
		help:       NewHelp(),
		keys:       DefaultKeymap,
		stepLines:  map[int][]string{},
	}
	if preselectedItem != nil {
		m.selected = *preselectedItem
		if !preselectedItem.hasDB {
			m.dropDB = false
		}
		if !preselectedItem.hasGtab {
			m.removeGtab = false
		}
		m.stage = washStageConfirm
	}
	return m, nil
}

func (m *washModel) Init() tea.Cmd { return m.spinner.Tick }

func (m *washModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = msg.Width
		m.list.SetSize(msg.Width, msg.Height-3)
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.stage = washStageAborted
			return m, tea.Quit
		}
		if msg.String() == "?" {
			m.help.ShowAll = !m.help.ShowAll
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
				m.pickWash(it)
				return m, nil
			}
		case "esc":
			m.stage = washStageAborted
			return m, tea.Quit
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			idx := int(k.String()[0] - '0' - 1)
			items := m.list.VisibleItems()
			if idx >= 0 && idx < len(items) {
				if it, ok := items[idx].(washItem); ok {
					m.pickWash(it)
					return m, nil
				}
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *washModel) pickWash(it washItem) {
	m.selected = it
	m.confirmArmed = false
	if !it.hasDB {
		m.dropDB = false
	}
	if !it.hasGtab {
		m.removeGtab = false
	}
	m.stage = washStageConfirm
}

// needsDoubleConfirm reports whether wash would destroy work that isn't safely
// in origin/<main> — unmerged commits (aheadCount) or uncommitted changes.
func (m *washModel) needsDoubleConfirm() bool {
	return m.selected.aheadCount > 0 || m.selected.dirty
}

func (m *washModel) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "up", "k":
			if m.confirmCursor > 0 {
				m.confirmCursor--
			}
		case "down", "j":
			if m.confirmCursor < m.visibleToggleCount()-1 {
				m.confirmCursor++
			}
		case " ", "tab":
			switch m.toggleAtCursor() {
			case "db":
				m.dropDB = !m.dropDB
			case "gtab":
				m.removeGtab = !m.removeGtab
			}
		case "enter":
			// Branches with unmerged/uncommitted work get force-deleted (git -D)
			// — require a second, deliberate enter before proceeding.
			if m.needsDoubleConfirm() && !m.confirmArmed {
				m.confirmArmed = true
				return m, nil
			}
			m.stage = washStageRunning
			plan := liftoff.WashPlan{
				Name:         m.selected.name,
				Branch:       m.selected.branch,
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
			m.confirmArmed = false
			m.stage = washStageSelect
		}
	}
	return m, nil
}

// washStepTitles must match RunWash's step order 1:1 so StepUpdate.Index lines
// up with m.stepStatuses. Skipped steps (db/gtab) render via their StepSkipped
// status, not a title suffix.
func washStepTitles(p liftoff.WashPlan) []string {
	branch := p.Branch
	if branch == "" {
		branch = p.Name
	}
	return []string{
		"stop running services",
		"remove worktree " + p.WorktreePath,
		"delete branch " + branch,
		"drop database " + liftoff.DBName(p.Name),
		"remove gtab workspace",
		"free port slot",
	}
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
			// Only worktree-remove (index 1) is fatal — matches RunWash.
			if u.Status == liftoff.StepFailed && u.Index == 1 {
				m.failed = true
				m.failureErr = u.Err
			}
		}
		return m, washNextUpdate(m.updates)
	}
	return m, nil
}

func (m *washModel) View() string {
	var body string
	switch m.stage {
	case washStageSelect:
		body = m.list.View()
	case washStageConfirm:
		body = m.viewConfirm()
	case washStageRunning:
		body = m.viewRunning()
	case washStageDone:
		body = m.viewDone()
	case washStageAborted:
		return StyleWarn.Render("aborted.\n")
	}
	return body + "\n" + m.help.View(m.keys)
}

// visibleToggles returns the toggle ids ("db", "gtab") that should
// appear given the worktree's actual state.
func (m *washModel) visibleToggles() []string {
	var out []string
	if m.selected.hasDB {
		out = append(out, "db")
	}
	if m.selected.hasGtab {
		out = append(out, "gtab")
	}
	return out
}

func (m *washModel) visibleToggleCount() int { return len(m.visibleToggles()) }

func (m *washModel) toggleAtCursor() string {
	v := m.visibleToggles()
	if m.confirmCursor < 0 || m.confirmCursor >= len(v) {
		return ""
	}
	return v[m.confirmCursor]
}

func (m *washModel) viewConfirm() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("kit wash — "+m.selected.name) + "\n\n")
	b.WriteString("worktree: " + m.selected.path + "\n")
	b.WriteString("branch:   " + m.selected.branch + "\n")
	if m.selected.dirty {
		b.WriteString(StyleWarn.Render("⚠ uncommitted changes will be lost") + "\n")
	}
	if m.selected.aheadCount > 0 {
		b.WriteString(StyleWarn.Render(fmt.Sprintf("⚠ %d commit(s) not in %s — will be permanently deleted (git -D)",
			m.selected.aheadCount, m.layout.MainBranch)) + "\n")
	}
	b.WriteString("\n")
	toggles := m.visibleToggles()
	if len(toggles) == 0 {
		b.WriteString(StyleDim.Render("nothing extra to clean up beyond worktree + branch") + "\n")
	}
	for i, id := range toggles {
		cursor := "  "
		if m.confirmCursor == i {
			cursor = "> "
		}
		box := "[ ]"
		if (id == "db" && m.dropDB) || (id == "gtab" && m.removeGtab) {
			box = StyleOK.Render("[x]")
		}
		label := ""
		switch id {
		case "db":
			label = "drop database " + liftoff.DBName(m.selected.name)
		case "gtab":
			label = "remove gtab workspace " + m.selected.name + ".applescript"
		}
		b.WriteString(cursor + box + " " + label + "\n")
	}
	b.WriteString("\n")
	if m.confirmArmed {
		b.WriteString(StyleWarn.Render("press enter again to permanently delete · esc to abort"))
	} else {
		b.WriteString(StyleHelp.Render("space: toggle · enter: confirm · backspace: back · esc: abort"))
	}
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

// RunWashTUI is the cobra entry point — full picker flow.
func RunWashTUI(layout liftoff.Layout) error { return RunWashTUIFor(layout, "") }

// RunWashTUIFor runs wash with an optional pre-selected worktree. Empty
// preselected falls back to the picker (the original behavior).
func RunWashTUIFor(layout liftoff.Layout, preselected string) error {
	if !layout.MasterIsRepo() {
		return fmt.Errorf("master repo not found at %s", layout.Master)
	}
	m, err := NewWashModelFor(layout, preselected)
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
	return nil
}
