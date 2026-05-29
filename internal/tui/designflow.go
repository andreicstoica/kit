package tui

import (
	"errors"
	"fmt"
	"os"
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
	name     string
	cloneDB  bool
	backend  bool
	symlink  bool
	graphite bool
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
				Description(databaseHelp(dbDisabled)+"\n\nDefault is No — DB clones take significant disk space. Say Yes if this worktree needs its own data.").
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

// minLeftWidth is the narrowest the step list may get before the orb panel
// is stacked underneath instead of beside it.
const minLeftWidth = 40

// designModel renders the post-form progress display.
type designModel struct {
	layout   liftoff.Layout
	answers  *designAnswers
	worktree string

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
			return m, m.stopwatch.Stop()
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
		if m.stepStatuses[i] == liftoff.StepSkipped {
			continue
		}
		var marker string
		switch m.stepStatuses[i] {
		case liftoff.StepRunning:
			marker = m.spinner.View()
		case liftoff.StepDone:
			marker = StyleOK.Render(Glyph("done"))
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
		}
		left.WriteString("\n" + StyleHelp.Render("press enter to continue"))
	}

	// Lay the orb panel beside the step list, but never let the (variable-
	// width) left content push the fixed-width orb past the terminal edge —
	// that clipped its right border. Cap the left panel to whatever space is
	// left after reserving the orb's full width; lipgloss MaxWidth truncates
	// each line so the orb always renders flush inside m.width. When the
	// terminal is too narrow to hold both side by side, stack the orb below.
	orbView := m.orb.View()
	orbW := lipgloss.Width(orbView)
	leftStyle := lipgloss.NewStyle().Padding(0, 2)
	var body string
	switch {
	case m.width <= 0:
		// Pre-WindowSizeMsg: no size yet, fall back to the natural layout.
		body = lipgloss.JoinHorizontal(lipgloss.Top, leftStyle.Render(left.String()), orbView)
	case m.width < orbW+minLeftWidth:
		// Too narrow for two columns — stack the orb under the steps.
		leftPanel := leftStyle.MaxWidth(m.width).Render(left.String())
		body = lipgloss.JoinVertical(lipgloss.Left, leftPanel, orbView)
	default:
		leftPanel := leftStyle.MaxWidth(m.width - orbW).Render(left.String())
		body = lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, orbView)
	}
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

// PickGtabLayout prompts for the Ghostty workspace layout. When
// includeSkip is true, adds a "Skip" option that returns
// liftoff.GtabLayout(""). Used by `kit design` (with skip) and
// `kit swap` after picking Ghostty (without skip).
func PickGtabLayout(includeSkip bool) (liftoff.GtabLayout, error) {
	opts := []SelectOption[liftoff.GtabLayout]{
		{Label: "Simple (2 tabs)", Value: liftoff.GtabSimple},
		{Label: "Detailed (5 tabs)", Value: liftoff.GtabDetailed},
	}
	if includeSkip {
		opts = append(opts, SelectOption[liftoff.GtabLayout]{Label: "Skip — don't open", Value: liftoff.GtabLayout("")})
	}
	return RunSelect(
		"Ghostty workspace layout",
		"Simple: 2 tabs (shell + combined logs). Detailed: 5 tabs with per-service splits.",
		opts, liftoff.GtabSimple,
	)
}

// offerNextSteps asks both follow-up yes/no questions first, then runs
// the chosen actions in order. Keeps the prompts grouped so the user
// finishes the questionnaire before anything else opens.
func offerNextSteps(layout liftoff.Layout, name string) error {
	fmt.Println()
	fmt.Println(StyleOK.Render(fmt.Sprintf("✓ %s is ready", name)))
	fmt.Println()

	// Same opener as `kit swap`: pick an editor, the Ghostty workspace, or
	// skip. Non-fatal — a failed/declined open still lets the play prompt run.
	if _, err := OpenWorktree(OpenRequest{
		Layout:    layout,
		Name:      name,
		Path:      layout.WorktreePath(name),
		OfferSkip: true,
	}); err != nil {
		fmt.Println(StyleErr.Render("open failed: " + err.Error()))
	}

	wantPlay := true
	if err := huh.NewConfirm().
		Title("Start dev servers?").
		Description("Runs `kit play` for this worktree (frontend + backend + celery on its slot's port band).").
		Affirmative("Yes").
		Negative("Skip").
		Value(&wantPlay).Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil
		}
		return err
	}

	if wantPlay {
		if err := RunPlayTUI(layout, PlayConfig{Name: name}); err != nil {
			fmt.Println(StyleErr.Render("play failed: " + err.Error()))
		}
	}
	return nil
}
