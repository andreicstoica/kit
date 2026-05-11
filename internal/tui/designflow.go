package tui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/stopwatch"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// designAnswers is the form payload from huh.
type designAnswers struct {
	name      string
	cloneDB   bool
	backend   bool
	symlink   bool
	graphite  bool
}

// runDesignForm presents the huh form, validates everything, and returns answers.
// If prefillName is non-empty (e.g. from `kit design voice-agent`), the name
// input is pre-populated so the user only has to confirm or edit.
func runDesignForm(layout liftoff.Layout, prefillName string) (*designAnswers, error) {
	a := &designAnswers{
		name:     prefillName,
		cloneDB:  false, // DB clones are heavy; opt-in only
		backend:  true,  // backend deps always installed (not prompted)
		symlink:  true,
		graphite: liftoff.HasGraphite(),
	}

	dbDisabled := !liftoff.HasPostgres()
	if dbDisabled {
		a.cloneDB = false
	}
	gtDisabled := !liftoff.HasGraphite()
	if gtDisabled {
		a.graphite = false
	}

	// Force-set huh theme to honor lipgloss adaptive colors.
	theme := huh.ThemeCharm()

	// One question per group — huh advances groups individually, so each
	// prompt occupies its own screen. Backend pip install isn't prompted
	// anymore; it's always run.
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Feature name").
				Description("kebab-case · used as initial branch, worktree, and ghostty tab layout names").
				Placeholder("voice-agent").
				CharLimit(60).
				Value(&a.name).
				Validate(func(s string) error {
					n := liftoff.Normalize(s)
					if err := liftoff.Validate(n); err != nil {
						return err
					}
					p := layout.WorktreePath(n)
					if _, err := os.Stat(p); err == nil {
						return fmt.Errorf("path exists: %s", p)
					}
					if _, err := os.Stat(layout.LegacyWorktreePath(n)); err == nil {
						return fmt.Errorf("legacy path exists: %s", layout.LegacyWorktreePath(n))
					}
					return nil
				}),
		),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Clone local database?").
				Description(databaseHelp(dbDisabled) + "\n\nDefault is No — DB clones take significant disk space. Say Yes if this worktree needs its own data.").
				Affirmative("Yes").
				Negative("No").
				Value(&a.cloneDB),
		),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Symlink frontend node_modules from master?").
				Description("Saves ~2GB + skips a 2-min yarn install. (You can run yarn install in the worktree later if you need worktree-specific deps.)").
				Affirmative("Yes").
				Negative("No").
				Value(&a.symlink),
		),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Track in graphite?").
				Description(graphiteHelp(gtDisabled)).
				Affirmative("Yes").
				Negative("No").
				Value(&a.graphite),
		),
	).WithTheme(theme).
		WithShowHelp(true).
		WithShowErrors(true)

	if err := form.Run(); err != nil {
		return nil, err
	}

	a.name = liftoff.Normalize(a.name)
	if dbDisabled {
		a.cloneDB = false
	}
	if gtDisabled {
		a.graphite = false
	}
	return a, nil
}

func databaseHelp(disabled bool) string {
	if disabled {
		return "[disabled — pg_dump not on PATH]"
	}
	return "`createdb liftoff_<name>` then `pg_dump | psql` clone, with `SQLALCHEMY_DATABASE_NAME` rewrite in `backend/.env`"
}

func graphiteHelp(disabled bool) string {
	if disabled {
		return "[disabled — gt not on PATH]"
	}
	return "`gt track --parent master` so the branch shows up in your stack"
}

// designModel renders the post-form progress display.
type designModel struct {
	layout    liftoff.Layout
	answers   *designAnswers
	worktree  string

	spinner       spinner.Model
	stopwatch     stopwatch.Model
	progress      progress.Model
	orb           Orb
	keys          KeyMap
	help          help.Model
	updates       <-chan liftoff.StepUpdate
	stepTitles    []string
	stepStatuses  []liftoff.StepStatus
	stepElapsed   []time.Duration
	currentLines  map[int][]string
	failed        bool
	failedAt      int
	failureErr    error
	allocatedSlot int
	done          bool

	width, height int
}

type designStepMsg struct {
	upd liftoff.StepUpdate
	ok  bool
}

func designNext(ch <-chan liftoff.StepUpdate) tea.Cmd {
	return func() tea.Msg {
		u, ok := <-ch
		return designStepMsg{upd: u, ok: ok}
	}
}

func (m *designModel) plan() liftoff.DressPlan {
	return liftoff.DressPlan{
		Name:            m.answers.name,
		Worktree:        m.worktree,
		CloneDB:         m.answers.cloneDB,
		BackendDeps:     m.answers.backend,
		SymlinkFrontend: m.answers.symlink,
		GraphiteTrack:   m.answers.graphite,
		Gtab:            true,
	}
}

func (m *designModel) Init() tea.Cmd {
	titles := m.layout.StepTitles(m.plan())
	m.stepTitles = titles
	m.stepStatuses = make([]liftoff.StepStatus, len(titles))
	m.stepElapsed = make([]time.Duration, len(titles))
	m.currentLines = map[int][]string{}
	m.updates = m.layout.RunDress(m.plan())
	return tea.Batch(
		m.spinner.Tick,
		m.stopwatch.Init(),
		m.orb.Init(),
		designNext(m.updates),
	)
}

func (m *designModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = msg.Width
		m.progress.Width = max(20, msg.Width-50)
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if msg.String() == "?" {
			m.help.ShowAll = !m.help.ShowAll
		}
		if m.done && (msg.Type == tea.KeyEnter || msg.Type == tea.KeyEsc || msg.String() == "q") {
			return m, tea.Quit
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case stopwatch.TickMsg, stopwatch.StartStopMsg, stopwatch.ResetMsg:
		var cmd tea.Cmd
		m.stopwatch, cmd = m.stopwatch.Update(msg)
		return m, cmd
	case orbTickMsg:
		var cmd tea.Cmd
		m.orb, cmd = m.orb.Update(msg)
		return m, cmd
	case designStepMsg:
		if !msg.ok {
			m.done = true
			return m, nil
		}
		u := msg.upd
		if u.Index >= 0 && u.Index < len(m.stepStatuses) {
			m.stepStatuses[u.Index] = u.Status
			if u.Elapsed > 0 {
				m.stepElapsed[u.Index] = u.Elapsed
			}
			if u.AllocatedSlot > 0 {
				m.allocatedSlot = u.AllocatedSlot
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
		// Drive overall progress bar from completed-step ratio.
		completed := 0
		total := 0
		for _, s := range m.stepStatuses {
			switch s {
			case liftoff.StepDone, liftoff.StepSkipped, liftoff.StepFailed:
				completed++
				total++
			default:
				total++
			}
		}
		var pcmd tea.Cmd
		if total > 0 {
			pcmd = m.progress.SetPercent(float64(completed) / float64(total))
		}
		return m, tea.Batch(designNext(m.updates), pcmd)
	case progress.FrameMsg:
		var cmd tea.Cmd
		var pm tea.Model
		pm, cmd = m.progress.Update(msg)
		m.progress = pm.(progress.Model)
		return m, cmd
	}
	return m, nil
}

func (m *designModel) View() string {
	var left strings.Builder
	emoji := liftoff.EmojiFor(m.answers.name)
	titlePrefix := "kit design — "
	if emoji != "" {
		titlePrefix += emoji + " "
	}
	left.WriteString(StyleTitle.Render(titlePrefix+m.answers.name) + "\n\n")

	left.WriteString(m.progress.View() + "  " + StyleDim.Render(m.stopwatch.View()) + "\n\n")

	for i, title := range m.stepTitles {
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
		left.WriteString(line + "\n")
		if m.stepStatuses[i] == liftoff.StepRunning {
			for _, ln := range m.currentLines[i] {
				left.WriteString(StyleDim.Render("       "+truncate(ln, m.width-8)) + "\n")
			}
		}
	}

	if m.done {
		left.WriteString("\n")
		if m.failed {
			left.WriteString(StyleErr.Render("✗ failed at: "+m.stepTitles[m.failedAt]) + "\n")
			if m.failureErr != nil {
				left.WriteString("  " + m.failureErr.Error() + "\n")
			}
			left.WriteString(StyleDim.Render("  inspect partial state at "+m.worktree) + "\n")
		} else {
			left.WriteString(StyleOK.Render("✓ ready") + "\n\n")
			if m.allocatedSlot > 0 {
				ports := liftoff.PortsForSlot(m.allocatedSlot)
				left.WriteString(fmt.Sprintf("slot:     %d\n", m.allocatedSlot))
				left.WriteString(fmt.Sprintf("ports:    app:%d admin:%d api:%d admin_be:%d\n",
					ports.App, ports.Admin, ports.API, ports.AdminBE))
			}
			if m.answers.cloneDB {
				left.WriteString("db:       " + liftoff.DBName(m.answers.name) + "\n")
			}
			left.WriteString("\n" + StyleHi.Render("next:") + "\n")
			left.WriteString("  kit play " + m.answers.name + "\n")
			left.WriteString("  kit warmup " + m.answers.name + "\n")
			left.WriteString("  cd " + m.worktree + "\n")
		}
		left.WriteString("\n" + StyleHelp.Render("press enter to exit"))
	}

	leftPanel := lipgloss.NewStyle().Padding(0, 2).Render(left.String())
	body := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, m.orb.View())
	footer := "\n" + m.help.View(m.keys)
	return body + footer
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

// RunDesignTUI is the cobra entry point: huh form → bubble tea progress.
// prefillName is empty for `kit design`, set for `kit design <name>`.
func RunDesignTUI(layout liftoff.Layout, prefillName string) error {
	if !layout.MasterIsRepo() {
		return fmt.Errorf("master repo not found at %s (set KIT_ROOT/KIT_MASTER_DIR)", layout.Master)
	}

	answers, err := runDesignForm(layout, prefillName)
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil
		}
		return err
	}
	if answers.name == "" {
		return errors.New("no name given")
	}

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorAccent)

	pb := progress.New(progress.WithDefaultGradient(), progress.WithoutPercentage())
	pb.Width = 30

	sw := stopwatch.NewWithInterval(time.Second)

	m := &designModel{
		layout:    layout,
		answers:   answers,
		worktree:  layout.WorktreePath(answers.name),
		spinner:   sp,
		progress:  pb,
		stopwatch: sw,
		orb:       NewOrb(),
		keys:      DefaultKeymap,
		help:      NewHelp(),
	}

	final, runErr := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if runErr != nil {
		return runErr
	}
	if dm, ok := final.(*designModel); ok && dm.failed {
		return errors.New("kit design reported a failure")
	}
	// Post-design "what's next?" prompt — offers to launch the gtab
	// workspace, start dev servers, and/or open URLs in the browser
	// so the user lands on a ready-to-go feature with one extra step
	// instead of having to remember three more commands.
	return offerNextSteps(layout, answers.name)
}

// RunDressTUI is kept as a back-compat alias.
func RunDressTUI(layout liftoff.Layout) error { return RunDesignTUI(layout, "") }

// offerNextSteps prompts after a successful design run. Multi-select
// over the three things people usually want next; runs each in
// sequence. Skips entirely if the user picks nothing.
func offerNextSteps(layout liftoff.Layout, name string) error {
	const (
		actGtab    = "gtab"
		actPlay    = "play"
		actBrowser = "browser"
	)
	var picks []string
	if err := huh.NewMultiSelect[string]().
		Title(fmt.Sprintf("✓ %s is ready. anything else?", name)).
		Description("space to toggle · enter to run · esc to skip").
		Options(
			huh.NewOption("Open Ghostty workspace  (`kit warmup`)", actGtab),
			huh.NewOption("Start dev servers       (`kit play`)", actPlay),
			huh.NewOption("Open frontend URLs in browser", actBrowser),
		).
		Value(&picks).Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil
		}
		return err
	}
	wantGtab := false
	wantPlay := false
	wantBrowser := false
	for _, p := range picks {
		switch p {
		case actGtab:
			wantGtab = true
		case actPlay:
			wantPlay = true
		case actBrowser:
			wantBrowser = true
		}
	}
	// gtab first so the workspace is open when servers boot.
	if wantGtab {
		if !layout.HasGtab(name) {
			if _, err := layout.WriteGtab(name, layout.WorktreePath(name)); err != nil {
				fmt.Println(StyleErr.Render("gtab write failed: " + err.Error()))
			}
		}
		if err := layout.LaunchGtab(name); err != nil {
			fmt.Println(StyleErr.Render("gtab launch failed: " + err.Error()))
		}
	}
	if wantPlay {
		if err := RunPlayTUI(layout, PlayConfig{Name: name}); err != nil {
			fmt.Println(StyleErr.Render("play failed: " + err.Error()))
		}
	}
	if wantBrowser {
		cfg, _ := liftoff.LoadConfig()
		slot := 0
		if cfg != nil {
			if m, ok := cfg.Worktrees[name]; ok {
				slot = m.Slot
			}
		}
		ports := liftoff.PortsForSlot(slot)
		for _, url := range []string{
			fmt.Sprintf("http://localhost:%d", ports.App),
			fmt.Sprintf("http://localhost:%d", ports.Admin),
		} {
			c := exec.Command("open", url)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			_ = c.Start()
		}
	}
	return nil
}
