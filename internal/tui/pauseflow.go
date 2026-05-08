package tui

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type pauseStage int

const (
	pauseStagePicker pauseStage = iota
	pauseStageConfirm
	pauseStageRun
	pauseStageDone
	pauseStageAborted
)

type pauseModel struct {
	layout liftoff.Layout
	stage  pauseStage

	picker  list.Model
	chosen  playWtItem // reuse the play picker item type
	running []liftoff.Service

	updates  <-chan liftoff.PlayUpdate
	statuses map[liftoff.Service]liftoff.StepStatus
	failed   bool

	spinner       spinner.Model
	help          help.Model
	keys          KeyMap
	width, height int

	preselectedName string
	onlyServices    []liftoff.Service
}

// NewPauseModel constructs the pause flow.
func NewPauseModel(layout liftoff.Layout, name string, only []liftoff.Service) (tea.Model, error) {
	m := &pauseModel{
		layout:          layout,
		stage:           pauseStagePicker,
		statuses:        map[liftoff.Service]liftoff.StepStatus{},
		preselectedName: name,
		onlyServices:    only,
	}
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorAccent)
	m.spinner = sp
	m.help = NewHelp()
	m.keys = DefaultKeymap

	if name != "" {
		path := layout.WorktreePath(name)
		if _, err := layout.ListWorktrees(); err != nil {
			return nil, err
		}
		m.chosen = playWtItem{name: name, path: path, emoji: liftoff.EmojiFor(name)}
		m.running = m.discoverRunning(name)
		if len(m.running) == 0 {
			return nil, fmt.Errorf("no services running for %s", name)
		}
		m.stage = pauseStageConfirm
		return m, nil
	}

	items, err := buildPauseItems(layout)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, errors.New("no running services to stop")
	}
	dlg := list.NewDefaultDelegate()
	dlg.Styles.SelectedTitle = dlg.Styles.SelectedTitle.Foreground(colorAccent).BorderForeground(colorAccent)
	pl := list.New(items, dlg, 0, 0)
	pl.Title = "kit pause — stop services"
	pl.Styles.Title = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	pl.SetShowHelp(true)
	pl.SetFilteringEnabled(true)
	m.picker = pl
	return m, nil
}

func (m *pauseModel) discoverRunning(name string) []liftoff.Service {
	st, _ := liftoff.LoadState()
	var slot int
	if st != nil {
		if meta, ok := st.Worktrees[name]; ok {
			slot = meta.Slot
		}
	}
	ports := liftoff.PortsForSlot(slot)
	var out []liftoff.Service
	for _, svc := range liftoff.AllServices {
		if liftoff.StatusOf(name, svc, ports).Alive {
			out = append(out, svc)
		}
	}
	if len(m.onlyServices) > 0 {
		filter := map[liftoff.Service]bool{}
		for _, s := range m.onlyServices {
			filter[s] = true
		}
		var trimmed []liftoff.Service
		for _, s := range out {
			if filter[s] {
				trimmed = append(trimmed, s)
			}
		}
		out = trimmed
	}
	return out
}

func buildPauseItems(layout liftoff.Layout) ([]list.Item, error) {
	wts, err := layout.ListWorktrees()
	if err != nil {
		return nil, err
	}
	st, _ := liftoff.LoadState()
	if st == nil {
		st = &liftoff.State{Worktrees: map[string]liftoff.WorktreeMeta{}}
	}
	type row struct {
		item playWtItem
	}
	var rows []row
	for _, w := range wts {
		if w.IsMaster(layout) || w.Bare {
			continue
		}
		name := w.Name()
		meta := st.Worktrees[name]
		ports := liftoff.PortsForSlot(meta.Slot)
		running := 0
		for _, svc := range liftoff.AllServices {
			if liftoff.StatusOf(name, svc, ports).Alive {
				running++
			}
		}
		if running == 0 {
			continue
		}
		rows = append(rows, row{item: playWtItem{
			name: name, path: w.Path, emoji: liftoff.EmojiFor(name),
			slot: meta.Slot, lastUsed: meta.LastUsed, running: running,
		}})
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].item.lastUsed.After(rows[j].item.lastUsed)
	})
	out := make([]list.Item, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.item)
	}
	return out, nil
}

func (m *pauseModel) Init() tea.Cmd { return m.spinner.Tick }

func (m *pauseModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = msg.Width
		if m.picker.Items() != nil {
			m.picker.SetSize(msg.Width, msg.Height-3)
		}
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.stage = pauseStageAborted
			return m, tea.Quit
		}
		if msg.String() == "?" {
			m.help.ShowAll = !m.help.ShowAll
		}
	}
	switch m.stage {
	case pauseStagePicker:
		return m.updatePicker(msg)
	case pauseStageConfirm:
		return m.updateConfirm(msg)
	case pauseStageRun:
		return m.updateRun(msg)
	case pauseStageDone, pauseStageAborted:
		if k, ok := msg.(tea.KeyMsg); ok {
			if k.Type == tea.KeyEnter || k.Type == tea.KeyEsc || k.String() == "q" {
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m *pauseModel) updatePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "enter":
			if it, ok := m.picker.SelectedItem().(playWtItem); ok {
				m.chosen = it
				m.running = m.discoverRunning(it.name)
				m.stage = pauseStageConfirm
				return m, nil
			}
		case "esc":
			m.stage = pauseStageAborted
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)
	return m, cmd
}

func (m *pauseModel) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "y", "Y", "enter":
			plan := liftoff.PausePlan{Worktree: m.chosen.name, Services: m.running}
			m.updates = m.layout.RunPause(plan)
			m.stage = pauseStageRun
			return m, tea.Batch(m.spinner.Tick, playNext(m.updates))
		case "n", "N", "esc":
			m.stage = pauseStageAborted
			return m, tea.Quit
		case "backspace":
			if m.preselectedName == "" {
				m.stage = pauseStagePicker
			}
		}
	}
	return m, nil
}

func (m *pauseModel) updateRun(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case playUpdMsg:
		if !msg.ok {
			m.stage = pauseStageDone
			return m, nil
		}
		u := msg.upd
		m.statuses[u.Service] = u.Status
		if u.Status == liftoff.StepFailed {
			m.failed = true
		}
		return m, playNext(m.updates)
	}
	return m, nil
}

func (m *pauseModel) View() string {
	var body string
	switch m.stage {
	case pauseStagePicker:
		body = m.picker.View()
	case pauseStageConfirm:
		body = m.viewConfirm()
	case pauseStageRun:
		body = m.viewRun()
	case pauseStageDone:
		body = m.viewDone()
	case pauseStageAborted:
		return StyleWarn.Render("aborted.\n")
	}
	return body + "\n" + m.help.View(m.keys)
}

func (m *pauseModel) viewConfirm() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("kit pause — "+m.chosen.name) + "\n\n")
	b.WriteString("services to stop:\n")
	for _, s := range m.running {
		b.WriteString("  " + StyleWarn.Render(Glyph("running")) + "  " + s.Label() + "\n")
	}
	b.WriteString("\n" + StyleHelp.Render("[Y]es stop · [n] cancel · backspace back · esc abort"))
	return b.String()
}

func (m *pauseModel) viewRun() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("kit pause — "+m.chosen.name) + "\n\n")
	for _, svc := range m.running {
		st := m.statuses[svc]
		var marker string
		switch st {
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
		b.WriteString(fmt.Sprintf("  %s  %s\n", marker, svc.Label()))
	}
	return b.String()
}

func (m *pauseModel) viewDone() string {
	if m.failed {
		return StyleErr.Render("✗ kit pause had failures") + "\n" + StyleHelp.Render("press enter to exit")
	}
	return StyleOK.Render("✓ "+m.chosen.name+" paused.") + "\n\n" + StyleHelp.Render("press enter to exit")
}

// RunPauseTUI is the cobra entry point.
func RunPauseTUI(layout liftoff.Layout, name string, only []liftoff.Service) error {
	m, err := NewPauseModel(layout, name, only)
	if err != nil {
		return err
	}
	final, runErr := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if runErr != nil {
		return runErr
	}
	if pm, ok := final.(*pauseModel); ok && pm.failed {
		return errors.New("kit pause reported a failure")
	}
	return nil
}

// PauseAll stops every service across every worktree, printing progress
// as plain text (no TUI). Used by `kit pause --all`.
func PauseAll(layout liftoff.Layout) error {
	st, err := liftoff.LoadState()
	if err != nil {
		return err
	}
	count := 0
	for name, meta := range st.Worktrees {
		ports := liftoff.PortsForSlot(meta.Slot)
		for _, svc := range liftoff.AllServices {
			s := liftoff.StatusOf(name, svc, ports)
			if !s.Alive {
				continue
			}
			fmt.Printf("  stopping %s/%s (pid %d)\n", name, svc.Label(), s.PID)
			_ = liftoff.StopService(name, svc)
			count++
		}
	}
	// Sweep orphans not in state.
	owner, pid := liftoff.FindCeleryOwner()
	if owner != "" {
		fmt.Printf("  stopping orphan celery for %s (pid %d)\n", owner, pid)
		_ = liftoff.StopService(owner, liftoff.SvcCelery)
		_ = liftoff.StopService(owner, liftoff.SvcBeat)
	}
	if count == 0 {
		fmt.Println("nothing to stop")
	}
	return nil
}
