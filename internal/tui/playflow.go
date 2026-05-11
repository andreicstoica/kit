package tui

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/stopwatch"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type playStage int

const (
	playStagePicker playStage = iota
	playStageToggle
	playStageAdoptPrompt
	playStageCeleryPrompt
	playStageRun
	playStageDone
	playStageAborted
)

type playWtItem struct {
	name       string
	path       string
	emoji      string
	slot       int
	lastUsed   time.Time
	running    int
	displayIdx int // 1-based for numeric quick-pick; 0 = no prefix
}

func (i playWtItem) Title() string {
	t := i.name
	if i.emoji != "" {
		t = i.emoji + " " + t
	}
	if i.displayIdx > 0 && i.displayIdx < 10 {
		t = StyleHi.Render(fmt.Sprintf("%d ", i.displayIdx)) + t
	}
	if i.slot > 0 {
		t += StyleDim.Render(fmt.Sprintf("  slot %d", i.slot))
	}
	if i.running > 0 {
		t += "  " + StyleOK.Render(fmt.Sprintf("%d running", i.running))
	}
	return t
}
func (i playWtItem) Description() string {
	return StyleDim.Render(i.path)
}
func (i playWtItem) FilterValue() string { return i.name }

type playModel struct {
	layout liftoff.Layout

	stage playStage

	// Picker stage
	picker  list.Model
	chosen  playWtItem

	// Toggle stage
	toggleSvcs   []liftoff.Service
	toggleOn     map[liftoff.Service]bool
	toggleCursor int

	// Celery confirm stage
	celeryVictim string
	celeryPID    int
	celeryAccept bool // true if user said yes (default Y)

	// Adopt prompt stage — fires when m.chosen has no slot yet.
	adoptBranch string
	adoptPath   string

	// Run stage
	runUpdates  <-chan liftoff.PlayUpdate
	runStatuses map[liftoff.Service]liftoff.StepStatus
	runMessages map[liftoff.Service]string
	runURLs     map[liftoff.Service]string
	runPIDs     map[liftoff.Service]int
	runOrder    []liftoff.Service
	failed      bool
	failureSvc  liftoff.Service
	failureErr  error

	spinner   spinner.Model
	stopwatch stopwatch.Model
	help      help.Model
	keys      KeyMap

	width, height int

	preselectedName string
	skipToggle      bool
	plan            liftoff.PlayPlan
}

// PlayConfig is what `kit play` was invoked with — preselected name (may be
// empty → picker), explicit service list (skips toggle screen), and a
// no-celery shortcut.
type PlayConfig struct {
	Name     string
	Only     []liftoff.Service
	NoCelery bool
}

// NewPlayModel constructs the initial play model. If cfg.Name is non-empty,
// the picker stage is skipped.
func NewPlayModel(layout liftoff.Layout, cfg PlayConfig) (tea.Model, error) {
	m := &playModel{
		layout:          layout,
		stage:           playStagePicker,
		toggleOn:        map[liftoff.Service]bool{},
		runStatuses:     map[liftoff.Service]liftoff.StepStatus{},
		runMessages:     map[liftoff.Service]string{},
		runURLs:         map[liftoff.Service]string{},
		runPIDs:         map[liftoff.Service]int{},
		preselectedName: cfg.Name,
	}
	for _, s := range liftoff.DefaultServices {
		m.toggleOn[s] = true
	}
	if cfg.NoCelery {
		m.toggleOn[liftoff.SvcCelery] = false
		m.toggleOn[liftoff.SvcBeat] = false
	}
	if len(cfg.Only) > 0 {
		// Override defaults with the explicit set.
		for _, s := range liftoff.AllServices {
			m.toggleOn[s] = false
		}
		for _, s := range cfg.Only {
			m.toggleOn[s] = true
		}
	}
	// UI toggle shows display services only — beat is collapsed into celery
	// (toggling celery flips both internally, see updateToggle).
	m.toggleSvcs = liftoff.DisplayServices
	name := cfg.Name
	only := cfg.Only

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorAccent)
	m.spinner = sp
	m.stopwatch = stopwatch.NewWithInterval(time.Second)
	m.help = NewHelp()
	m.keys = DefaultKeymap

	if name != "" {
		// Skip picker, validate the worktree exists.
		path := layout.WorktreePath(name)
		if _, err := os.Stat(path); err != nil {
			path = layout.LegacyWorktreePath(name)
			if _, err2 := os.Stat(path); err2 != nil {
				return nil, fmt.Errorf("worktree not found: %s", name)
			}
		}
		st, _ := liftoff.LoadState()
		var slot int
		if st != nil {
			if meta, ok := st.Worktrees[name]; ok {
				slot = meta.Slot
			}
		}
		m.chosen = playWtItem{name: name, path: path, slot: slot, emoji: liftoff.EmojiFor(name)}
		m.stage = playStageToggle
		if len(only) > 0 {
			// Skip toggle screen — Init() will fire the transition.
			m.skipToggle = true
		}
		return m, nil
	}

	// Build picker.
	items, err := buildPlayItems(layout)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, errors.New("no worktrees found — run `kit design` first")
	}
	dlg := list.NewDefaultDelegate()
	dlg.Styles.SelectedTitle = dlg.Styles.SelectedTitle.Foreground(colorAccent).BorderForeground(colorAccent)
	dlg.Styles.SelectedDesc = dlg.Styles.SelectedDesc.Foreground(colorAccent).BorderForeground(colorAccent)
	pl := list.New(items, dlg, 0, 0)
	pl.Title = "kit play — pick a kit to put on"
	pl.Styles.Title = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	pl.SetShowHelp(true)
	pl.SetFilteringEnabled(true)
	m.picker = pl

	return m, nil
}

func buildPlayItems(layout liftoff.Layout) ([]list.Item, error) {
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
		if w.Bare {
			continue
		}
		name := w.Name()
		emoji := liftoff.EmojiFor(name)
		if w.IsMaster(layout) {
			name = "master"
			emoji = "🏠"
		}
		meta := st.Worktrees[name]
		running := 0
		ports := liftoff.PortsForSlot(meta.Slot)
		for _, svc := range liftoff.AllServices {
			s := liftoff.StatusOf(name, svc, ports)
			if s.Alive {
				running++
			}
		}
		rows = append(rows, row{item: playWtItem{
			name:     name,
			path:     w.Path,
			emoji:    emoji,
			slot:     meta.Slot,
			lastUsed: meta.LastUsed,
			running:  running,
		}})
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].item.lastUsed.After(rows[j].item.lastUsed)
	})
	out := make([]list.Item, 0, len(rows))
	for i, r := range rows {
		r.item.displayIdx = i + 1
		out = append(out, r.item)
	}
	return out, nil
}

// transitionAfterToggle resolves the slot, prompts to adopt if the
// worktree is unknown, then shows the celery prompt or jumps to run.
func (m *playModel) transitionAfterToggle() tea.Cmd {
	return func() tea.Msg {
		st, err := liftoff.LoadConfig()
		if err != nil {
			return playSetupErrMsg{err}
		}
		meta, ok := st.Worktrees[m.chosen.name]
		// Master is special: slot 0 is its assigned slot (master defaults), not a
		// missing allocation. Adopt prompt only fires for unknown feature
		// worktrees.
		needsAdopt := !ok
		if !needsAdopt && m.chosen.name != "master" && meta.Slot == 0 {
			needsAdopt = true
		}
		if needsAdopt {
			// Unadopted — bail to a confirm prompt. The user explicitly approves
			// before kit allocates a slot + writes metadata.
			branch, path := findBranchAndPath(m.layout, m.chosen.name)
			return playAdoptPromptMsg{branch: branch, path: path}
		}
		st.TouchLastUsed(m.chosen.name)
		_ = st.Save()
		m.chosen.slot = meta.Slot

		// Build the plan.
		var selected []liftoff.Service
		for _, s := range liftoff.AllServices {
			if m.toggleOn[s] {
				selected = append(selected, s)
			}
		}
		ports := liftoff.PortsForSlot(m.chosen.slot)
		plan := liftoff.PlayPlan{
			Worktree:     m.chosen.name,
			WorktreePath: m.chosen.path,
			Slot:         m.chosen.slot,
			Ports:        ports,
			Services:     selected,
		}

		// Detect celery owner conflict.
		if m.toggleOn[liftoff.SvcCelery] {
			owner, pid := liftoff.FindCeleryOwner()
			if owner != "" && owner != m.chosen.name {
				return playCeleryConflictMsg{victim: owner, pid: pid, plan: plan}
			}
		}
		return playReadyMsg{plan: plan}
	}
}

// findBranchAndPath looks up the on-disk branch + path for a kit name via
// git worktree list. Returns ("", "") if not found.
func findBranchAndPath(layout liftoff.Layout, name string) (string, string) {
	wts, err := layout.ListWorktrees()
	if err != nil {
		return "", ""
	}
	for _, w := range wts {
		if w.IsMaster(layout) || w.Bare {
			continue
		}
		if w.Name() == name {
			return w.Branch, w.Path
		}
	}
	return "", ""
}

// Messages
type playReadyMsg struct{ plan liftoff.PlayPlan }
type playAdoptPromptMsg struct {
	branch string
	path   string
}
type playCeleryConflictMsg struct {
	victim string
	pid    int
	plan   liftoff.PlayPlan
}
type playSetupErrMsg struct{ err error }
type playUpdMsg struct {
	upd liftoff.PlayUpdate
	ok  bool
}

func playNext(ch <-chan liftoff.PlayUpdate) tea.Cmd {
	return func() tea.Msg {
		u, ok := <-ch
		return playUpdMsg{upd: u, ok: ok}
	}
}

func (m *playModel) Init() tea.Cmd {
	if m.skipToggle {
		return tea.Batch(m.spinner.Tick, m.transitionAfterToggle())
	}
	return m.spinner.Tick
}

func (m *playModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = msg.Width
		if m.picker.Items() != nil {
			m.picker.SetSize(msg.Width, msg.Height-3)
		}
	case stopwatch.TickMsg, stopwatch.StartStopMsg, stopwatch.ResetMsg:
		var cmd tea.Cmd
		m.stopwatch, cmd = m.stopwatch.Update(msg)
		return m, cmd
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.stage = playStageAborted
			return m, tea.Quit
		}
		if msg.String() == "?" {
			m.help.ShowAll = !m.help.ShowAll
		}
	case playSetupErrMsg:
		m.failed = true
		m.failureErr = msg.err
		m.stage = playStageDone
		return m, nil
	case playReadyMsg:
		m.plan = msg.plan
		m.stage = playStageRun
		m.runOrder = msg.plan.Services
		m.runUpdates = m.layout.RunPlay(msg.plan)
		return m, tea.Batch(m.spinner.Tick, m.stopwatch.Init(), playNext(m.runUpdates))
	case playCeleryConflictMsg:
		m.celeryVictim = msg.victim
		m.celeryPID = msg.pid
		m.plan = msg.plan
		m.celeryAccept = true // default Y
		m.stage = playStageCeleryPrompt
		return m, nil
	case playAdoptPromptMsg:
		m.adoptBranch = msg.branch
		m.adoptPath = msg.path
		m.stage = playStageAdoptPrompt
		return m, nil
	}

	switch m.stage {
	case playStagePicker:
		return m.updatePicker(msg)
	case playStageToggle:
		return m.updateToggle(msg)
	case playStageAdoptPrompt:
		return m.updateAdopt(msg)
	case playStageCeleryPrompt:
		return m.updateCelery(msg)
	case playStageRun:
		return m.updateRun(msg)
	case playStageDone, playStageAborted:
		if k, ok := msg.(tea.KeyMsg); ok {
			if k.Type == tea.KeyEnter || k.Type == tea.KeyEsc || k.String() == "q" {
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m *playModel) updatePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "enter":
			if it, ok := m.picker.SelectedItem().(playWtItem); ok {
				m.chosen = it
				m.stage = playStageToggle
				return m, nil
			}
		case "esc":
			m.stage = playStageAborted
			return m, tea.Quit
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			idx := int(k.String()[0] - '0' - 1)
			items := m.picker.VisibleItems()
			if idx >= 0 && idx < len(items) {
				if it, ok := items[idx].(playWtItem); ok {
					m.chosen = it
					m.stage = playStageToggle
					return m, nil
				}
			}
		}
	}
	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)
	return m, cmd
}

func (m *playModel) updateToggle(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "up", "k":
			if m.toggleCursor > 0 {
				m.toggleCursor--
			}
		case "down", "j":
			if m.toggleCursor < len(m.toggleSvcs)-1 {
				m.toggleCursor++
			}
		case " ", "tab":
			s := m.toggleSvcs[m.toggleCursor]
			m.toggleOn[s] = !m.toggleOn[s]
			// Celery toggle controls beat too — they're paired.
			if s == liftoff.SvcCelery {
				m.toggleOn[liftoff.SvcBeat] = m.toggleOn[liftoff.SvcCelery]
			}
		case "enter":
			return m, m.transitionAfterToggle()
		case "esc":
			m.stage = playStageAborted
			return m, tea.Quit
		case "backspace":
			if m.preselectedName == "" {
				m.stage = playStagePicker
			}
		}
	}
	return m, nil
}

func (m *playModel) updateAdopt(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "y", "Y", "enter":
		// Run shared adopt; on success, retry transitionAfterToggle which
		// will now find the new slot in config.
		path := m.adoptPath
		branch := m.adoptBranch
		if path == "" {
			path = m.chosen.path
		}
		opts := liftoff.AdoptOptions{
			SymlinkNodeModules: false, // play is hot path; don't surprise-rewrite frontend
			WriteGtab:          false,
			GraphiteTrack:      false,
		}
		if _, err := m.layout.Adopt(m.chosen.name, branch, path, opts, nil); err != nil {
			return m, func() tea.Msg { return playSetupErrMsg{err} }
		}
		m.stage = playStageToggle
		return m, m.transitionAfterToggle()
	case "n", "N", "esc":
		m.stage = playStageAborted
		return m, tea.Quit
	}
	return m, nil
}

func (m *playModel) updateCelery(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "y", "Y", "enter":
			m.celeryAccept = true
			m.plan.ReplaceCelery = true
			m.plan.ReplaceVictim = m.celeryVictim
			m.stage = playStageRun
			m.runOrder = m.plan.Services
			m.runUpdates = m.layout.RunPlay(m.plan)
			return m, tea.Batch(m.spinner.Tick, playNext(m.runUpdates))
		case "n", "N":
			// Drop celery + beat from the plan, then proceed.
			filtered := make([]liftoff.Service, 0, len(m.plan.Services))
			for _, s := range m.plan.Services {
				if s != liftoff.SvcCelery && s != liftoff.SvcBeat {
					filtered = append(filtered, s)
				}
			}
			m.plan.Services = filtered
			m.stage = playStageRun
			m.runOrder = m.plan.Services
			m.runUpdates = m.layout.RunPlay(m.plan)
			return m, tea.Batch(m.spinner.Tick, playNext(m.runUpdates))
		case "esc":
			m.stage = playStageAborted
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *playModel) updateRun(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case playUpdMsg:
		if !msg.ok {
			m.stage = playStageDone
			return m, nil
		}
		u := msg.upd
		m.runStatuses[u.Service] = u.Status
		if u.URL != "" {
			m.runURLs[u.Service] = u.URL
		}
		if u.PID > 0 {
			m.runPIDs[u.Service] = u.PID
		}
		if u.Status == liftoff.StepFailed {
			m.failed = true
			m.failureSvc = u.Service
			m.failureErr = u.Err
			m.runMessages[u.Service] = u.Err.Error()
		}
		return m, playNext(m.runUpdates)
	}
	return m, nil
}

func (m *playModel) View() string {
	var body string
	switch m.stage {
	case playStagePicker:
		body = m.picker.View()
	case playStageToggle:
		body = m.viewToggle()
	case playStageAdoptPrompt:
		body = m.viewAdopt()
	case playStageCeleryPrompt:
		body = m.viewCelery()
	case playStageRun:
		body = m.viewRun()
	case playStageDone:
		body = m.viewDone()
	case playStageAborted:
		return StyleWarn.Render("aborted.\n")
	}
	footer := "\n" + m.help.View(m.keys)
	return body + footer
}

func (m *playModel) viewToggle() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("kit play — "+m.chosen.name) + "\n\n")
	if m.chosen.slot > 0 {
		b.WriteString(StyleDim.Render(fmt.Sprintf("slot %d  ports %d/%d/%d/%d\n", m.chosen.slot,
			3000+m.chosen.slot*10, 3001+m.chosen.slot*10,
			9000+m.chosen.slot*10, 9001+m.chosen.slot*10)))
	} else {
		b.WriteString(StyleDim.Render("slot will be allocated when you press Enter\n"))
	}
	b.WriteString("\n")
	ports := liftoff.PortsForSlot(m.chosen.slot)
	for i, svc := range m.toggleSvcs {
		cursor := "  "
		if i == m.toggleCursor {
			cursor = "> "
		}
		box := "[ ]"
		if m.toggleOn[svc] {
			box = StyleOK.Render("[x]")
		}
		label := svc.Label()
		if svc == liftoff.SvcMCP {
			label += StyleDim.Render(" (opt-in)")
		}
		// Show whether the service is currently running so the user can tell
		// "kit will (re)start these" apart from "these are already alive".
		state := StyleDim.Render("○ stopped")
		if liftoff.IsServiceAlive(m.chosen.name, svc, ports) {
			state = StyleOK.Render("● running")
		}
		b.WriteString(cursor + box + " " + padRight(label, 12) + "  " + state + "\n")
	}
	b.WriteString("\n" + StyleHelp.Render("space/tab: toggle · enter: continue · backspace: back · esc: abort"))
	return b.String()
}

func (m *playModel) viewAdopt() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("kit play — adopt first") + "\n\n")
	b.WriteString(fmt.Sprintf("worktree %s isn't adopted yet.\n", StyleHi.Render(m.chosen.name)))
	if m.adoptBranch != "" {
		b.WriteString(StyleDim.Render("  branch: "+m.adoptBranch) + "\n")
	}
	if m.adoptPath != "" {
		b.WriteString(StyleDim.Render("  path:   "+m.adoptPath) + "\n")
	}
	b.WriteString("\nadopt now? this allocates a port slot + records it in config.toml.\n")
	b.WriteString("\n" + StyleHelp.Render("[Y]es adopt and continue · [n] abort · esc abort"))
	return b.String()
}

func (m *playModel) viewCelery() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("kit play — celery already running") + "\n\n")
	b.WriteString(fmt.Sprintf("celery is running for %s (pid %d).\n", StyleHi.Render(m.celeryVictim), m.celeryPID))
	b.WriteString("\nstarting " + StyleHi.Render(m.chosen.name) + "'s celery will kill it.\n\n")
	b.WriteString(StyleHelp.Render("[Y]es replace · [n] skip celery for this run · esc abort"))
	return b.String()
}

func (m *playModel) viewRun() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("kit play — "+m.chosen.name) + "  " +
		StyleDim.Render(m.stopwatch.View()) + "\n\n")
	if m.plan.ReplaceCelery {
		b.WriteString(StyleDim.Render(fmt.Sprintf("replaced %s's celery\n", m.plan.ReplaceVictim)))
	}
	for _, svc := range m.runOrder {
		st := m.runStatuses[svc]
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
		line := fmt.Sprintf("  %s  %s", marker, svc.Label())
		if u, ok := m.runURLs[svc]; ok {
			line += "  " + StyleDim.Render(u)
		} else if pid, ok := m.runPIDs[svc]; ok && st == liftoff.StepDone {
			line += StyleDim.Render(fmt.Sprintf("  pid %d", pid))
		}
		b.WriteString(line + "\n")
		if msg := m.runMessages[svc]; msg != "" && st == liftoff.StepFailed {
			b.WriteString(StyleErr.Render("       "+msg) + "\n")
		}
	}
	return b.String()
}

func (m *playModel) viewDone() string {
	var b strings.Builder
	if m.failed {
		b.WriteString(StyleErr.Render("✗ kit play failed at "+m.failureSvc.Label()) + "\n")
		if m.failureErr != nil {
			b.WriteString("  " + m.failureErr.Error() + "\n")
		}
		runDir, _ := liftoff.RunDir(m.chosen.name)
		b.WriteString(StyleDim.Render("  logs: "+runDir) + "\n")
	} else {
		b.WriteString(StyleOK.Render("✓ "+m.chosen.name+" playing — slot "+fmt.Sprint(m.chosen.slot)) + "\n\n")
		for _, svc := range m.runOrder {
			line := fmt.Sprintf("  %-15s ", svc.Label()+":")
			if u, ok := m.runURLs[svc]; ok {
				line += u
			} else if pid, ok := m.runPIDs[svc]; ok {
				line += fmt.Sprintf("pid %d", pid)
			}
			b.WriteString(line + "\n")
		}
		runDir, _ := liftoff.RunDir(m.chosen.name)
		b.WriteString("\n" + StyleDim.Render("logs: "+runDir) + "\n")
	}
	b.WriteString("\n" + StyleHelp.Render("press enter to exit"))
	return b.String()
}

// RunPlayTUI is the cobra entry point.
func RunPlayTUI(layout liftoff.Layout, cfg PlayConfig) error {
	if !layout.MasterIsRepo() {
		return fmt.Errorf("master repo not found at %s", layout.Master)
	}
	m, err := NewPlayModel(layout, cfg)
	if err != nil {
		return err
	}
	final, runErr := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if runErr != nil {
		return runErr
	}
	if pm, ok := final.(*playModel); ok && pm.failed {
		return errors.New("kit play reported a failure")
	}
	return nil
}
