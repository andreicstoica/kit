package tui

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// dressStage identifies the current wizard screen.
type dressStage int

const (
	stageName dressStage = iota
	stageToggleDB
	stageToggleBackend
	stageToggleFrontend
	stageToggleGraphite
	stageToggleGtab
	stageReview
	stageRunning
	stageDone
	stageAborted
)

type toggle struct {
	label    string
	help     string
	on       bool
	disabled bool
	reason   string // why disabled
}

type dressModel struct {
	layout liftoff.Layout

	stage dressStage

	// stageName
	nameInput  textinput.Model
	nameError  string

	// resolved name + toggles
	name     string
	worktree string
	db       toggle
	backend  toggle
	frontend toggle
	graphite toggle
	gtab     toggle

	// run state
	spinner       spinner.Model
	updates       <-chan liftoff.StepUpdate
	stepTitles    []string
	stepStatuses  []liftoff.StepStatus
	stepElapsed   []time.Duration
	currentLines  map[int][]string // last few log lines per step
	failed        bool
	failedAt      int
	failureErr    error
	totalElapsed  time.Duration
	startTime     time.Time

	width  int
	height int
}

// NewDressModel constructs the initial wizard.
func NewDressModel(layout liftoff.Layout) tea.Model {
	ti := textinput.New()
	ti.Placeholder = "voice-agent"
	ti.Prompt = "> "
	ti.CharLimit = 60
	ti.Width = 40
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorAccent)

	return &dressModel{
		layout:       layout,
		stage:        stageName,
		nameInput:    ti,
		spinner:      sp,
		db:           toggle{label: "Clone local DB", help: "pg_dump liftoff → liftoff_<name>; updates backend/.env", on: true},
		backend:      toggle{label: "Install backend deps", help: "pip install -r requirements.txt -r requirements_test.txt", on: true},
		frontend:     toggle{label: "Install frontend deps", help: "yarn install in frontend/app and frontend/admin", on: true},
		graphite:     toggle{label: "Track in graphite", help: "gt track --parent master so this branch joins your stack", on: true},
		gtab:         toggle{label: "Create gtab workspace", help: "AppleScript launcher for ghostty (4 tabs, splits)", on: true},
		currentLines: map[int][]string{},
	}
}

func (m *dressModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.spinner.Tick)
}

// stepUpdateMsg is a single channel read.
type stepUpdateMsg struct {
	upd liftoff.StepUpdate
	ok  bool
}

func nextUpdate(ch <-chan liftoff.StepUpdate) tea.Cmd {
	return func() tea.Msg {
		u, ok := <-ch
		return stepUpdateMsg{upd: u, ok: ok}
	}
}

func (m *dressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.stage = stageAborted
			return m, tea.Quit
		}
	}

	switch m.stage {
	case stageName:
		return m.updateName(msg)
	case stageToggleDB, stageToggleBackend, stageToggleFrontend, stageToggleGraphite, stageToggleGtab:
		return m.updateToggle(msg)
	case stageReview:
		return m.updateReview(msg)
	case stageRunning:
		return m.updateRunning(msg)
	case stageDone, stageAborted:
		if k, ok := msg.(tea.KeyMsg); ok {
			if k.Type == tea.KeyEnter || k.Type == tea.KeyEsc || k.String() == "q" {
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m *dressModel) updateName(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.Type {
		case tea.KeyEnter:
			raw := m.nameInput.Value()
			n, err := liftoff.NormalizeAndValidate(raw)
			if err != nil {
				m.nameError = err.Error()
				return m, nil
			}
			path := m.layout.WorktreePath(n)
			if _, e := os.Stat(path); e == nil {
				m.nameError = "path already exists: " + path
				return m, nil
			}
			if _, e := os.Stat(m.layout.LegacyWorktreePath(n)); e == nil {
				m.nameError = "legacy path exists: " + m.layout.LegacyWorktreePath(n)
				return m, nil
			}
			m.name = n
			m.worktree = path
			m.nameError = ""
			// Pre-disable toggles based on env capability.
			if !liftoff.HasPostgres() {
				m.db.disabled = true
				m.db.on = false
				m.db.reason = "pg_dump not on PATH"
			}
			if !liftoff.HasGraphite() {
				m.graphite.disabled = true
				m.graphite.on = false
				m.graphite.reason = "gt not on PATH"
			}
			m.stage = stageToggleDB
			return m, nil
		case tea.KeyEsc:
			m.stage = stageAborted
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)
	return m, cmd
}

func (m *dressModel) currentToggle() *toggle {
	switch m.stage {
	case stageToggleDB:
		return &m.db
	case stageToggleBackend:
		return &m.backend
	case stageToggleFrontend:
		return &m.frontend
	case stageToggleGraphite:
		return &m.graphite
	case stageToggleGtab:
		return &m.gtab
	}
	return nil
}

func (m *dressModel) updateToggle(msg tea.Msg) (tea.Model, tea.Cmd) {
	t := m.currentToggle()
	if t == nil {
		return m, nil
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "y", "Y":
			if !t.disabled {
				t.on = true
			}
			m.stage++
		case "n", "N":
			t.on = false
			m.stage++
		case "tab", " ":
			if !t.disabled {
				t.on = !t.on
			}
		case "enter":
			m.stage++
		case "esc":
			m.stage = stageAborted
			return m, tea.Quit
		case "backspace":
			if m.stage > stageToggleDB {
				m.stage--
			}
		}
	}
	return m, nil
}

func (m *dressModel) plan() liftoff.DressPlan {
	return liftoff.DressPlan{
		Name:          m.name,
		Worktree:      m.worktree,
		CloneDB:       m.db.on,
		BackendDeps:   m.backend.on,
		FrontendDeps:  m.frontend.on,
		GraphiteTrack: m.graphite.on,
		Gtab:          m.gtab.on,
	}
}

func (m *dressModel) updateReview(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "enter":
			m.stage = stageRunning
			titles := m.layout.StepTitles(m.plan())
			m.stepTitles = titles
			m.stepStatuses = make([]liftoff.StepStatus, len(titles))
			m.stepElapsed = make([]time.Duration, len(titles))
			m.startTime = time.Now()
			m.updates = m.layout.RunDress(m.plan())
			return m, tea.Batch(m.spinner.Tick, nextUpdate(m.updates))
		case "esc", "q":
			m.stage = stageAborted
			return m, tea.Quit
		case "backspace":
			m.stage = stageToggleGtab
		}
	}
	return m, nil
}

func (m *dressModel) updateRunning(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case stepUpdateMsg:
		if !msg.ok {
			m.totalElapsed = time.Since(m.startTime)
			m.stage = stageDone
			return m, nil
		}
		u := msg.upd
		if u.Index >= 0 && u.Index < len(m.stepStatuses) {
			m.stepStatuses[u.Index] = u.Status
			if u.Elapsed > 0 {
				m.stepElapsed[u.Index] = u.Elapsed
			}
			if u.Line != "" {
				lines := m.currentLines[u.Index]
				lines = append(lines, u.Line)
				if len(lines) > 4 {
					lines = lines[len(lines)-4:]
				}
				m.currentLines[u.Index] = lines
			}
			if u.Status == liftoff.StepFailed {
				m.failed = true
				m.failedAt = u.Index
				m.failureErr = u.Err
			}
		}
		return m, nextUpdate(m.updates)
	}
	return m, nil
}

func (m *dressModel) View() string {
	switch m.stage {
	case stageName:
		return m.viewName()
	case stageToggleDB, stageToggleBackend, stageToggleFrontend, stageToggleGraphite, stageToggleGtab:
		return m.viewToggle()
	case stageReview:
		return m.viewReview()
	case stageRunning:
		return m.viewRunning()
	case stageDone:
		return m.viewDone()
	case stageAborted:
		return StyleWarn.Render("aborted.\n")
	}
	return ""
}

func (m *dressModel) viewName() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("kit dress") + StyleDim.Render(" — put on a fresh kit") + "\n\n")
	b.WriteString("Feature name (leading 'liftoff-' will be stripped):\n")
	b.WriteString(m.nameInput.View() + "\n")
	if m.nameError != "" {
		b.WriteString(StyleErr.Render("  ✗ "+m.nameError) + "\n")
	} else if v := m.nameInput.Value(); v != "" {
		if n, err := liftoff.NormalizeAndValidate(v); err == nil {
			b.WriteString(StyleDim.Render("  → " + m.layout.WorktreePath(n)) + "\n")
		}
	}
	b.WriteString("\n" + StyleHelp.Render("enter: continue · esc: abort"))
	return b.String()
}

func (m *dressModel) viewToggle() string {
	t := m.currentToggle()
	idx := int(m.stage - stageToggleDB)
	total := 5
	var b strings.Builder
	b.WriteString(StyleTitle.Render(fmt.Sprintf("kit dress — step %d/%d", idx+2, total+2)) + "\n\n")
	b.WriteString(StyleHi.Render(t.label) + "\n")
	b.WriteString(StyleDim.Render("  "+t.help) + "\n\n")
	box := "[ ]"
	if t.on {
		box = StyleOK.Render("[x]")
	}
	if t.disabled {
		box = StyleErr.Render("[-]")
	}
	b.WriteString(box + "  " + t.label + "\n")
	if t.disabled {
		b.WriteString(StyleErr.Render("  disabled: "+t.reason) + "\n")
	}
	b.WriteString("\n" + StyleHelp.Render("y/n: set · space/tab: toggle · enter: continue · backspace: back · esc: abort"))
	return b.String()
}

func (m *dressModel) viewReview() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("kit dress — review") + "\n\n")
	b.WriteString("name:     " + StyleHi.Render(m.name) + "\n")
	b.WriteString("worktree: " + m.worktree + "\n")
	b.WriteString("branch:   " + m.name + "\n\n")
	for _, line := range m.layout.StepTitles(m.plan()) {
		b.WriteString("  " + line + "\n")
	}
	skipped := []string{}
	for _, t := range []toggle{m.db, m.backend, m.frontend, m.graphite, m.gtab} {
		if !t.on {
			skipped = append(skipped, t.label)
		}
	}
	if len(skipped) > 0 {
		b.WriteString("\n" + StyleDim.Render("skipping: "+strings.Join(skipped, ", ")) + "\n")
	}
	b.WriteString("\n" + StyleHelp.Render("enter: run · backspace: back · esc: abort"))
	return b.String()
}

func (m *dressModel) viewRunning() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("kit dress — "+m.name) + "\n\n")
	for i, title := range m.stepTitles {
		st := m.stepStatuses[i].String()
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
		line := fmt.Sprintf("  %s  %s", marker, title)
		if m.stepElapsed[i] > 0 && m.stepStatuses[i] == liftoff.StepDone {
			line += StyleDim.Render(fmt.Sprintf("  (%s)", m.stepElapsed[i].Round(10*time.Millisecond)))
		}
		b.WriteString(line + "\n")
		if m.stepStatuses[i] == liftoff.StepRunning {
			for _, ln := range m.currentLines[i] {
				b.WriteString(StyleDim.Render("       "+truncate(ln, m.width-8)) + "\n")
			}
		}
		_ = st
	}
	if m.failed {
		b.WriteString("\n" + StyleErr.Render("failed: "+m.failureErr.Error()) + "\n")
	}
	return b.String()
}

func (m *dressModel) viewDone() string {
	var b strings.Builder
	if m.failed {
		b.WriteString(StyleErr.Render("✗ kit dress failed") + "\n\n")
		b.WriteString("step: " + m.stepTitles[m.failedAt] + "\n")
		b.WriteString("error: " + m.failureErr.Error() + "\n\n")
		b.WriteString("Inspect partial state at " + m.worktree + " before retrying.\n")
	} else {
		b.WriteString(StyleOK.Render("✓ kit dressed: "+m.name) + "\n\n")
		b.WriteString("worktree: " + m.worktree + "\n")
		b.WriteString("branch:   " + m.name + "\n")
		if m.db.on {
			b.WriteString("db:       " + liftoff.DBName(m.name) + "\n")
		}
		b.WriteString("\n" + StyleHi.Render("next:") + "\n")
		b.WriteString("  cd " + m.worktree + "\n")
		if m.gtab.on {
			b.WriteString("  kit warmup " + m.name + "    # launch ghostty workspace\n")
		}
	}
	b.WriteString("\n" + StyleHelp.Render("press enter to exit"))
	return b.String()
}

func truncate(s string, w int) string {
	if w <= 0 || len(s) <= w {
		return s
	}
	if w < 4 {
		return s[:w]
	}
	return s[:w-1] + "…"
}

// RunDressTUI is the entry point invoked by the cobra command.
func RunDressTUI(layout liftoff.Layout) error {
	if !layout.MasterIsRepo() {
		return fmt.Errorf("master repo not found at %s (set KIT_ROOT/KIT_MASTER_DIR if your layout differs)", layout.Master)
	}
	m := NewDressModel(layout)
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return err
	}
	if dm, ok := final.(*dressModel); ok && dm.failed {
		return errors.New("kit dress reported a failure")
	}
	return nil
}
